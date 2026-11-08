package usecase

import (
	"context"

	"receh/internal/domain"
	"receh/internal/platform/keymutex"
)

type TransactionInput struct {
	WalletID   int64         `json:"walletId"`
	CategoryID int64         `json:"categoryId"`
	Type       domain.TxType `json:"type"`
	Name       string        `json:"name"`
	Amount     int64         `json:"amount"`
	OccurredOn string        `json:"occurredOn"`
}

// TransactionService is the only path that moves money in response to a person.
// Every change holds the mutex of each wallet it touches, so "check balance,
// then spend" is one indivisible step per wallet.
type TransactionService struct {
	txs        domain.TransactionRepository
	wallets    domain.WalletRepository
	categories domain.CategoryRepository
	clock      domain.Clock
	locks      *keymutex.Map
	notify     notifier
}

func NewTransactionService(t domain.TransactionRepository, w domain.WalletRepository, c domain.CategoryRepository, clock domain.Clock, locks *keymutex.Map, n domain.ChangeNotifier) *TransactionService {
	return &TransactionService{txs: t, wallets: w, categories: c, clock: clock, locks: locks, notify: notifier{n}}
}

func (s *TransactionService) validate(ctx context.Context, userID int64, in *TransactionInput, allowArchivedWallet int64) error {
	name, err := cleanName("name", in.Name, 80, "Beri nama kegiatannya, misalnya \"Makan siang\".")
	if err != nil {
		return err
	}
	in.Name = name
	if !in.Type.Valid() {
		return domain.Invalid("type", "Pilih pemasukan atau pengeluaran.")
	}
	if err := validAmount("amount", in.Amount); err != nil {
		return err
	}
	on, err := domain.ParseDate(in.OccurredOn)
	if err != nil {
		return domain.Invalid("occurredOn", "Tanggal tidak valid.")
	}
	// One day of slack covers a client in a timezone ahead of the server's.
	today, _ := domain.ParseDate(s.clock.Today())
	if on.After(today.AddDate(0, 0, 1)) {
		return domain.Invalid("occurredOn", "Tanggal tidak boleh di masa depan. Untuk rencana, pakai Transaksi Rutin.")
	}
	w, err := s.wallets.ByID(ctx, userID, in.WalletID)
	if err != nil {
		if e := notFoundOr(err, ""); e != err {
			return domain.Invalid("walletId", "Pilih dompet.")
		}
		return err
	}
	if w.Archived && w.ID != allowArchivedWallet {
		return domain.Invalid("walletId", "Dompet ini sudah diarsipkan.")
	}
	c, err := s.categories.ByID(ctx, userID, in.CategoryID)
	if err != nil {
		if e := notFoundOr(err, ""); e != err {
			return domain.Invalid("categoryId", "Pilih kategori.")
		}
		return err
	}
	if c.Type != in.Type {
		return domain.Invalid("categoryId", "Kategori ini bukan untuk jenis transaksi tersebut.")
	}
	return nil
}

func (s *TransactionService) Create(ctx context.Context, userID int64, in TransactionInput) (*domain.Transaction, error) {
	defer s.notify.changed(userID)
	if err := s.validate(ctx, userID, &in, 0); err != nil {
		return nil, err
	}
	unlock := s.locks.Lock(in.WalletID)
	defer unlock()
	t := &domain.Transaction{
		UserID: userID, WalletID: in.WalletID, CategoryID: in.CategoryID,
		Type: in.Type, Name: in.Name, Amount: in.Amount, OccurredOn: in.OccurredOn,
	}
	if err := s.txs.Create(ctx, t); err != nil {
		return nil, err
	}
	return s.txs.ByID(ctx, userID, t.ID)
}

func (s *TransactionService) Update(ctx context.Context, userID, id int64, in TransactionInput) (*domain.Transaction, error) {
	defer s.notify.changed(userID)
	current, err := s.txs.ByID(ctx, userID, id)
	if err != nil {
		return nil, notFoundOr(err, "Transaksi tidak ditemukan.")
	}
	if current.TransferID != nil {
		return nil, errPartOfTransfer
	}
	if err := s.validate(ctx, userID, &in, current.WalletID); err != nil {
		return nil, err
	}
	unlock := s.locks.Lock(current.WalletID, in.WalletID)
	defer unlock()
	// Re-read under the lock: the row may have changed while we waited.
	old, err := s.txs.ByID(ctx, userID, id)
	if err != nil {
		return nil, notFoundOr(err, "Transaksi tidak ditemukan.")
	}
	if old.WalletID != current.WalletID {
		return nil, domain.Conflict("STALE", "Transaksi baru saja berubah. Muat ulang lalu coba lagi.")
	}
	updated := *old
	updated.WalletID, updated.CategoryID, updated.Type = in.WalletID, in.CategoryID, in.Type
	updated.Name, updated.Amount, updated.OccurredOn = in.Name, in.Amount, in.OccurredOn
	if err := s.txs.Update(ctx, old, &updated); err != nil {
		return nil, friendlyReversal(err)
	}
	return s.txs.ByID(ctx, userID, id)
}

func (s *TransactionService) Delete(ctx context.Context, userID, id int64) error {
	defer s.notify.changed(userID)
	current, err := s.txs.ByID(ctx, userID, id)
	if err != nil {
		return notFoundOr(err, "Transaksi tidak ditemukan.")
	}
	if current.TransferID != nil {
		return errPartOfTransfer
	}
	unlock := s.locks.Lock(current.WalletID)
	defer unlock()
	t, err := s.txs.ByID(ctx, userID, id)
	if err != nil {
		return notFoundOr(err, "Transaksi tidak ditemukan.")
	}
	return friendlyReversal(s.txs.Delete(ctx, t))
}

// errPartOfTransfer protects the legs and fee of a transfer: changing one alone
// would leave money that left one wallet without arriving in the other.
var errPartOfTransfer = domain.Conflict("PART_OF_TRANSFER", "Transaksi ini bagian dari transfer antar dompet. Ubah atau hapus lewat transfernya.")

// friendlyReversal explains the less obvious case: removing past income whose
// money has already been spent.
func friendlyReversal(err error) error {
	if err == domain.ErrInsufficient {
		return domain.Conflict("INSUFFICIENT_BALANCE", "Perubahan ini membuat saldo dompet menjadi minus, jadi tidak bisa disimpan.")
	}
	return err
}

func (s *TransactionService) List(ctx context.Context, userID int64, f domain.TxFilter) (domain.Page[domain.Transaction], error) {
	return s.txs.List(ctx, userID, f)
}
