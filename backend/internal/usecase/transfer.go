package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"receh/internal/domain"
	"receh/internal/platform/keymutex"
)

// FeeCategoryName is the expense category transfer fees are booked under.
const FeeCategoryName = "Biaya admin"

// MaxTransferFee is a sanity cap. Real fees in Indonesia are a few thousand
// rupiah; a much larger number is almost certainly a typo in the amount field.
const MaxTransferFee int64 = 100_000

type TransferInput struct {
	FromWalletID int64  `json:"fromWalletId"`
	ToWalletID   int64  `json:"toWalletId"`
	Amount       int64  `json:"amount"`
	Fee          int64  `json:"fee"`
	Note         string `json:"note"`
	OccurredOn   string `json:"occurredOn"`
}

// TransferService moves money between a user's own wallets. Like
// TransactionService it holds the mutex of every wallet involved, taken in
// ascending order by keymutex, so A→B and B→A at the same moment cannot
// deadlock and neither can overdraw.
type TransferService struct {
	transfers  domain.TransferRepository
	wallets    domain.WalletRepository
	categories domain.CategoryRepository
	clock      domain.Clock
	locks      *keymutex.Map
	notify     notifier
}

func NewTransferService(t domain.TransferRepository, w domain.WalletRepository, c domain.CategoryRepository, clock domain.Clock, locks *keymutex.Map, n domain.ChangeNotifier) *TransferService {
	return &TransferService{transfers: t, wallets: w, categories: c, clock: clock, locks: locks, notify: notifier{n}}
}

// validate checks the input and returns both wallets. allowArchived lists
// wallets an edit may keep even though they were archived since.
func (s *TransferService) validate(ctx context.Context, userID int64, in *TransferInput, allowArchived ...int64) (from, to *domain.Wallet, err error) {
	in.Note = strings.Join(strings.Fields(in.Note), " ")
	if utf8.RuneCountInString(in.Note) > 80 {
		return nil, nil, domain.Invalid("note", "Catatan maksimal 80 karakter.")
	}
	if err := validAmount("amount", in.Amount); err != nil {
		return nil, nil, err
	}
	if in.Fee < 0 {
		return nil, nil, domain.Invalid("fee", "Biaya admin tidak boleh negatif.")
	}
	if in.Fee > MaxTransferFee {
		return nil, nil, domain.Invalid("fee", "Biaya admin maksimal "+formatRp(MaxTransferFee)+". Periksa lagi nominalnya.")
	}
	on, err := domain.ParseDate(in.OccurredOn)
	if err != nil {
		return nil, nil, domain.Invalid("occurredOn", "Tanggal tidak valid.")
	}
	today, _ := domain.ParseDate(s.clock.Today())
	if on.After(today.AddDate(0, 0, 1)) {
		return nil, nil, domain.Invalid("occurredOn", "Tanggal tidak boleh di masa depan.")
	}
	if in.FromWalletID == in.ToWalletID {
		return nil, nil, domain.Invalid("toWalletId", "Pilih dompet tujuan yang berbeda dari dompet asal.")
	}
	load := func(id int64, field, missing string) (*domain.Wallet, error) {
		w, err := s.wallets.ByID(ctx, userID, id)
		if errors.Is(err, domain.ErrNoRows) {
			return nil, domain.Invalid(field, missing)
		}
		if err != nil {
			return nil, err
		}
		if w.Archived && !slices.Contains(allowArchived, w.ID) {
			return nil, domain.Invalid(field, "Dompet "+w.Name+" sudah diarsipkan.")
		}
		return w, nil
	}
	if from, err = load(in.FromWalletID, "fromWalletId", "Pilih dompet asal."); err != nil {
		return nil, nil, err
	}
	if to, err = load(in.ToWalletID, "toWalletId", "Pilih dompet tujuan."); err != nil {
		return nil, nil, err
	}
	return from, to, nil
}

// feeCategory finds the fee category, creating it for accounts made before
// transfers existed or whose owner deleted it.
func (s *TransferService) feeCategory(ctx context.Context, userID int64) (int64, error) {
	c, err := s.categories.ByName(ctx, userID, domain.TxExpense, FeeCategoryName)
	if err == nil {
		return c.ID, nil
	}
	if !errors.Is(err, domain.ErrNoRows) {
		return 0, err
	}
	created := &domain.Category{UserID: userID, Name: FeeCategoryName, Type: domain.TxExpense, Color: "c0"}
	if err := s.categories.Create(ctx, created); err != nil {
		// Lost a race with a parallel request: the other one created it.
		if c, again := s.categories.ByName(ctx, userID, domain.TxExpense, FeeCategoryName); again == nil {
			return c.ID, nil
		}
		return 0, err
	}
	return created.ID, nil
}

func (s *TransferService) build(ctx context.Context, userID int64, in TransferInput, from, to *domain.Wallet) (*domain.Transfer, error) {
	t := &domain.Transfer{
		UserID: userID, FromWalletID: from.ID, FromWalletName: from.Name, ToWalletID: to.ID, ToWalletName: to.Name,
		Amount: in.Amount, Fee: in.Fee, Note: in.Note, OccurredOn: in.OccurredOn,
	}
	if t.Fee > 0 {
		id, err := s.feeCategory(ctx, userID)
		if err != nil {
			return nil, err
		}
		t.FeeCategoryID = id
	}
	return t, nil
}

// shortfall explains a refused transfer with the numbers a person needs:
// what was required including the fee, and what the wallet holds.
func shortfall(w *domain.Wallet, need, fee int64) error {
	msg := "Saldo " + w.Name + " tidak cukup. Dibutuhkan " + formatRp(need)
	if fee > 0 {
		msg += " (termasuk biaya admin " + formatRp(fee) + ")"
	}
	return domain.Conflict("INSUFFICIENT_BALANCE", msg+", saldo saat ini "+formatRp(w.Balance)+".")
}

func (s *TransferService) Create(ctx context.Context, userID int64, in TransferInput) (*domain.Transfer, error) {
	defer s.notify.changed(userID)
	if _, _, err := s.validate(ctx, userID, &in); err != nil {
		return nil, err
	}
	unlock := s.locks.Lock(in.FromWalletID, in.ToWalletID)
	defer unlock()
	// Re-read under the lock so the balance check sees every earlier payment.
	from, to, err := s.validate(ctx, userID, &in)
	if err != nil {
		return nil, err
	}
	t, err := s.build(ctx, userID, in, from, to)
	if err != nil {
		return nil, err
	}
	if from.Balance < t.Total() {
		return nil, shortfall(from, t.Total(), t.Fee)
	}
	if err := s.transfers.Create(ctx, t); err != nil {
		if errors.Is(err, domain.ErrInsufficient) {
			return nil, shortfall(from, t.Total(), t.Fee)
		}
		return nil, err
	}
	return s.transfers.ByID(ctx, userID, t.ID)
}

func (s *TransferService) Get(ctx context.Context, userID, id int64) (*domain.Transfer, error) {
	t, err := s.transfers.ByID(ctx, userID, id)
	if err != nil {
		return nil, notFoundOr(err, "Transfer tidak ditemukan.")
	}
	return t, nil
}

func (s *TransferService) Update(ctx context.Context, userID, id int64, in TransferInput) (*domain.Transfer, error) {
	defer s.notify.changed(userID)
	current, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	keep := []int64{current.FromWalletID, current.ToWalletID}
	if _, _, err := s.validate(ctx, userID, &in, keep...); err != nil {
		return nil, err
	}
	unlock := s.locks.Lock(current.FromWalletID, current.ToWalletID, in.FromWalletID, in.ToWalletID)
	defer unlock()
	old, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if old.FromWalletID != current.FromWalletID || old.ToWalletID != current.ToWalletID {
		return nil, domain.Conflict("STALE", "Transfer baru saja berubah. Muat ulang lalu coba lagi.")
	}
	from, to, err := s.validate(ctx, userID, &in, keep...)
	if err != nil {
		return nil, err
	}
	updated, err := s.build(ctx, userID, in, from, to)
	if err != nil {
		return nil, err
	}
	if err := s.transfers.Update(ctx, old, updated); err != nil {
		if errors.Is(err, domain.ErrInsufficient) {
			return nil, domain.Conflict("INSUFFICIENT_BALANCE",
				"Perubahan ini membuat saldo salah satu dompet menjadi minus, jadi tidak bisa disimpan.")
		}
		return nil, notFoundOr(err, "Transfer tidak ditemukan.")
	}
	return s.transfers.ByID(ctx, userID, id)
}

func (s *TransferService) Delete(ctx context.Context, userID, id int64) error {
	defer s.notify.changed(userID)
	current, err := s.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	unlock := s.locks.Lock(current.FromWalletID, current.ToWalletID)
	defer unlock()
	t, err := s.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	if err := s.transfers.Delete(ctx, t); err != nil {
		if errors.Is(err, domain.ErrInsufficient) {
			return domain.Conflict("INSUFFICIENT_BALANCE",
				"Uang dari transfer ini sudah terpakai di "+t.ToWalletName+", jadi transfer tidak bisa dihapus.")
		}
		return err
	}
	return nil
}

// FeeHint is the fee to prefill for a route.
type FeeHint struct {
	Fee   int64 `json:"fee"`
	Found bool  `json:"found"`
}

func (s *TransferService) LastFee(ctx context.Context, userID, fromID, toID int64) (*FeeHint, error) {
	fee, found, err := s.transfers.LastFee(ctx, userID, fromID, toID)
	if err != nil {
		return nil, err
	}
	return &FeeHint{Fee: fee, Found: found}, nil
}

// formatRp writes 101000 as "Rp 101.000", matching the frontend.
func formatRp(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	digits := []byte(itoa64(v))
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteByte(d)
	}
	if neg {
		return "-Rp " + b.String()
	}
	return "Rp " + b.String()
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
