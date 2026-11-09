package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"receh/internal/domain"
	"receh/internal/platform/keymutex"
)

// EntryService makes recording faster: one-line quick entry, name suggestions
// from the user's own history, and importing bank/e-wallet statements (CSV).
type EntryService struct {
	entries    domain.EntryRepository
	wallets    domain.WalletRepository
	categories domain.CategoryRepository
	clock      domain.Clock
	locks      *keymutex.Map
	plans      PlanLimits
	notify     notifier
}

func NewEntryService(e domain.EntryRepository, w domain.WalletRepository, c domain.CategoryRepository, clock domain.Clock,
	locks *keymutex.Map, plans PlanLimits, n domain.ChangeNotifier) *EntryService {
	return &EntryService{entries: e, wallets: w, categories: c, clock: clock, locks: locks, plans: plans, notify: notifier{n}}
}

// MaxImportRows bounds one import; a month of student spending is far less.
const MaxImportRows = 1000

// Parse reads a one-line entry and completes it from history: if "Nasi padang"
// was recorded before, its category and wallet are reused when the text
// itself does not say.
func (s *EntryService) Parse(ctx context.Context, userID int64, text string) (*domain.QuickDraft, error) {
	if strings.TrimSpace(text) == "" {
		return nil, domain.Invalid("text", "Ketik dulu, misalnya \"makan siang 25rb gopay\".")
	}
	if utf8.RuneCountInString(text) > 200 {
		return nil, domain.Invalid("text", "Terlalu panjang.")
	}
	var wallets []domain.Wallet
	var cats []domain.Category
	err := parallel(
		func() (e error) { wallets, e = s.wallets.List(ctx, userID, false); return },
		func() (e error) { cats, e = s.categories.List(ctx, userID); return },
	)
	if err != nil {
		return nil, err
	}
	today, _ := domain.ParseDate(s.clock.Today())
	d := domain.ParseQuick(text, wallets, cats, today)
	if d.Name != "" && (d.CategoryID == 0 || d.WalletID == 0) {
		past, err := s.entries.Suggest(ctx, userID, d.Name, true, 1)
		if err != nil {
			return nil, err
		}
		if len(past) == 1 && past[0].Type == d.Type {
			if d.CategoryID == 0 {
				d.CategoryID = past[0].CategoryID
			}
			if d.WalletID == 0 && walletActive(wallets, past[0].WalletID) {
				d.WalletID = past[0].WalletID
			}
		}
	}
	return &d, nil
}

func walletActive(ws []domain.Wallet, id int64) bool {
	for _, w := range ws {
		if w.ID == id {
			return true
		}
	}
	return false
}

func (s *EntryService) Suggest(ctx context.Context, userID int64, prefix string) ([]domain.TxSuggestion, error) {
	prefix = strings.TrimSpace(prefix)
	if utf8.RuneCountInString(prefix) < 2 {
		return []domain.TxSuggestion{}, nil
	}
	return s.entries.Suggest(ctx, userID, prefix, false, 6)
}

// ---- Statement import -----------------------------------------------------------

type ImportRow struct {
	Line       int           `json:"line"`
	Date       string        `json:"date"`
	Name       string        `json:"name"`
	Amount     int64         `json:"amount"`
	Type       domain.TxType `json:"type"`
	CategoryID int64         `json:"categoryId"`
	Duplicate  bool          `json:"duplicate"`
}

type ImportPreview struct {
	Headers []string                `json:"headers"`
	Mapping domain.StatementMapping `json:"mapping"`
	Skipped int                     `json:"skipped"`
	Rows    []ImportRow             `json:"rows"`
}

func (s *EntryService) requireImport(ctx context.Context, userID int64) error {
	limits, err := s.plans.Limits(ctx, userID)
	if err != nil {
		return err
	}
	if !limits.CSVImport {
		return domain.PlanLimit("Impor mutasi CSV adalah fitur Premium.")
	}
	return nil
}

func (s *EntryService) importWallet(ctx context.Context, userID, walletID int64) (*domain.Wallet, error) {
	w, err := s.wallets.ByID(ctx, userID, walletID)
	if errors.Is(err, domain.ErrNoRows) {
		return nil, domain.Invalid("walletId", "Pilih dompet tujuan impor.")
	}
	if err != nil {
		return nil, err
	}
	if w.Archived {
		return nil, domain.Invalid("walletId", "Dompet ini sudah diarsipkan.")
	}
	return w, nil
}

// Preview parses a statement and proposes a category for every row, marking
// rows that already exist in the wallet (same date, type, amount and name).
func (s *EntryService) Preview(ctx context.Context, userID, walletID int64, csv string, mapping *domain.StatementMapping) (*ImportPreview, error) {
	if err := s.requireImport(ctx, userID); err != nil {
		return nil, err
	}
	if _, err := s.importWallet(ctx, userID, walletID); err != nil {
		return nil, err
	}
	today, _ := domain.ParseDate(s.clock.Today())
	parsed, err := domain.ParseStatement([]byte(csv), mapping, today)
	if errors.Is(err, domain.ErrStatementUnreadable) {
		return nil, domain.Invalid("file", "Berkas ini bukan CSV yang bisa dibaca. Ekspor mutasi sebagai CSV dari aplikasi bank atau e-wallet.")
	}
	if err != nil {
		return nil, err
	}
	if len(parsed.Rows) > MaxImportRows {
		return nil, domain.Invalid("file", "Maksimal "+itoa(MaxImportRows)+" baris sekali impor. Pisahkan berkasnya per bulan.")
	}
	out := &ImportPreview{Headers: parsed.Headers, Mapping: parsed.Mapping, Skipped: parsed.Skipped, Rows: make([]ImportRow, 0, len(parsed.Rows))}
	if len(parsed.Rows) == 0 {
		return out, nil
	}
	cats, err := s.categories.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	from, to := parsed.Rows[0].Date, parsed.Rows[0].Date
	for _, r := range parsed.Rows {
		from, to = min(from, r.Date), max(to, r.Date)
	}
	existing, err := s.entries.ExistingKeys(ctx, userID, walletID, from, to)
	if err != nil {
		return nil, err
	}
	fallback := map[domain.TxType]int64{}
	for _, c := range cats {
		if c.Name == "Lainnya" {
			fallback[c.Type] = c.ID
		}
	}
	memo := map[string]int64{}
	for _, r := range parsed.Rows {
		row := ImportRow{Line: r.Line, Date: r.Date, Name: r.Name, Amount: r.Amount, Type: r.Type}
		row.Duplicate = existing[domain.DuplicateKey(r.Date, r.Type, r.Amount, r.Name)]
		key := string(r.Type) + "|" + strings.ToLower(r.Name)
		if id, ok := memo[key]; ok {
			row.CategoryID = id
		} else {
			row.CategoryID = s.guessCategory(ctx, userID, r, cats, fallback[r.Type])
			memo[key] = row.CategoryID
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// guessCategory: the category used last time for this exact name, else the
// keyword guess, else "Lainnya" of that type.
func (s *EntryService) guessCategory(ctx context.Context, userID int64, r domain.StatementRow, cats []domain.Category, fallback int64) int64 {
	if past, err := s.entries.Suggest(ctx, userID, r.Name, true, 1); err == nil && len(past) == 1 && past[0].Type == r.Type && past[0].CategoryID != 0 {
		return past[0].CategoryID
	}
	d := domain.ParseQuick(r.Name, nil, cats, s.today())
	if d.CategoryID != 0 && d.Type == r.Type {
		return d.CategoryID
	}
	if d.Type != r.Type { // the keyword guessed the other type; retry within the row's type
		for _, c := range cats {
			if c.Type == r.Type && strings.Contains(strings.ToLower(r.Name), strings.ToLower(c.Name)) {
				return c.ID
			}
		}
	}
	return fallback
}

func (s *EntryService) today() time.Time {
	t, _ := domain.ParseDate(s.clock.Today())
	return t
}

type ImportCommit struct {
	WalletID int64       `json:"walletId"`
	FileName string      `json:"fileName"`
	Rows     []ImportRow `json:"rows"`
}

// Commit records the chosen rows as one batch: all or nothing, and undoable.
func (s *EntryService) Commit(ctx context.Context, userID int64, in ImportCommit) (*domain.ImportBatch, error) {
	defer s.notify.changed(userID)
	if err := s.requireImport(ctx, userID); err != nil {
		return nil, err
	}
	w, err := s.importWallet(ctx, userID, in.WalletID)
	if err != nil {
		return nil, err
	}
	if len(in.Rows) == 0 {
		return nil, domain.Invalid("rows", "Pilih minimal satu baris untuk diimpor.")
	}
	if len(in.Rows) > MaxImportRows {
		return nil, domain.Invalid("rows", "Maksimal "+itoa(MaxImportRows)+" baris sekali impor.")
	}
	cats, err := s.categories.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	catType := map[int64]domain.TxType{}
	for _, c := range cats {
		catType[c.ID] = c.Type
	}
	today := s.today()
	txs := make([]domain.Transaction, 0, len(in.Rows))
	for i, r := range in.Rows {
		where := "Baris " + itoa(i+1) + ": "
		if !r.Type.Valid() {
			return nil, domain.Invalid("rows", where+"jenis tidak valid.")
		}
		if err := validAmount("amount", r.Amount); err != nil {
			return nil, domain.Invalid("rows", where+"nominal tidak valid.")
		}
		d, err := domain.ParseDate(r.Date)
		if err != nil || d.After(today.AddDate(0, 0, 1)) {
			return nil, domain.Invalid("rows", where+"tanggal tidak valid.")
		}
		if t, ok := catType[r.CategoryID]; !ok || t != r.Type {
			return nil, domain.Invalid("rows", where+"kategori tidak cocok dengan jenisnya.")
		}
		name := strings.Join(strings.Fields(r.Name), " ")
		if name == "" {
			name = "Transaksi impor"
		}
		if utf8.RuneCountInString(name) > 80 {
			name = string([]rune(name)[:80])
		}
		txs = append(txs, domain.Transaction{UserID: userID, WalletID: w.ID, CategoryID: r.CategoryID, Type: r.Type,
			Name: name, Amount: r.Amount, OccurredOn: d.Format(domain.DateLayout)})
	}
	fileName := strings.TrimSpace(in.FileName)
	if utf8.RuneCountInString(fileName) > 120 {
		fileName = string([]rune(fileName)[:120])
	}
	unlock := s.locks.Lock(w.ID)
	defer unlock()
	b := &domain.ImportBatch{UserID: userID, WalletID: w.ID, WalletName: w.Name, FileName: fileName}
	if err := s.entries.CreateBatch(ctx, b, txs); err != nil {
		if errors.Is(err, domain.ErrInsufficient) {
			return nil, domain.Conflict("INSUFFICIENT_BALANCE", "Saldo "+w.Name+" sekarang tidak cukup untuk menampung pengeluaran di impor ini. "+
				"Mutasi bank biasanya sudah termasuk di saldo; sesuaikan saldo dompet dulu (catat pemasukan penyesuaian), lalu impor lagi.")
		}
		return nil, err
	}
	return b, nil
}

func (s *EntryService) Batches(ctx context.Context, userID int64) ([]domain.ImportBatch, error) {
	return s.entries.Batches(ctx, userID)
}

func (s *EntryService) Undo(ctx context.Context, userID, batchID int64) error {
	defer s.notify.changed(userID)
	err := s.entries.UndoBatch(ctx, userID, batchID)
	if errors.Is(err, domain.ErrNoRows) {
		return domain.NotFound("Impor tidak ditemukan.")
	}
	if errors.Is(err, domain.ErrInsufficient) {
		return domain.Conflict("INSUFFICIENT_BALANCE", "Uang dari impor ini sudah terpakai, jadi impor tidak bisa dibatalkan tanpa membuat saldo minus.")
	}
	return err
}
