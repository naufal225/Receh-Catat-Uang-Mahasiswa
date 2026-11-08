package usecase

import (
	"context"

	"receh/internal/domain"
	"receh/internal/platform/keymutex"
)

type WalletInput struct {
	Name    string            `json:"name"`
	Type    domain.WalletType `json:"type"`
	Color   string            `json:"color"`
	Balance int64             `json:"balance"`
}

type WalletService struct {
	wallets   domain.WalletRepository
	recurring domain.RecurringRepository
	locks     *keymutex.Map
	notify    notifier
}

func NewWalletService(w domain.WalletRepository, r domain.RecurringRepository, locks *keymutex.Map, n domain.ChangeNotifier) *WalletService {
	return &WalletService{wallets: w, recurring: r, locks: locks, notify: notifier{n}}
}

func (in *WalletInput) validate() error {
	name, err := cleanName("name", in.Name, 40, "Nama dompet wajib diisi.")
	if err != nil {
		return err
	}
	in.Name = name
	if !in.Type.Valid() {
		return domain.Invalid("type", "Pilih jenis dompet.")
	}
	return validColor(in.Color)
}

func (s *WalletService) Create(ctx context.Context, userID int64, in WalletInput) (*domain.Wallet, error) {
	defer s.notify.changed(userID)
	if err := in.validate(); err != nil {
		return nil, err
	}
	if in.Balance < 0 {
		return nil, domain.Invalid("balance", "Saldo awal tidak boleh negatif.")
	}
	if in.Balance > MaxAmount {
		return nil, domain.Invalid("balance", "Saldo awal terlalu besar.")
	}
	w := &domain.Wallet{UserID: userID, Name: in.Name, Type: in.Type, Color: in.Color, Balance: in.Balance}
	if err := s.wallets.Create(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *WalletService) List(ctx context.Context, userID int64) ([]domain.Wallet, error) {
	return s.wallets.List(ctx, userID, false)
}

// Update changes how a wallet looks. Balance is ignored on purpose: money only
// moves through transactions, so the history always explains the balance.
func (s *WalletService) Update(ctx context.Context, userID, id int64, in WalletInput) (*domain.Wallet, error) {
	defer s.notify.changed(userID)
	if err := in.validate(); err != nil {
		return nil, err
	}
	w, err := s.wallets.ByID(ctx, userID, id)
	if err != nil {
		return nil, notFoundOr(err, "Dompet tidak ditemukan.")
	}
	w.Name, w.Type, w.Color = in.Name, in.Type, in.Color
	if err := s.wallets.Update(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// Remove deletes an unused wallet. A wallet with history is archived instead, so
// past reports stay accurate. It returns true when the wallet was archived.
func (s *WalletService) Remove(ctx context.Context, userID, id int64) (archived bool, err error) {
	defer s.notify.changed(userID)
	unlock := s.locks.Lock(id)
	defer unlock()
	if _, err := s.wallets.ByID(ctx, userID, id); err != nil {
		return false, notFoundOr(err, "Dompet tidak ditemukan.")
	}
	used, err := s.wallets.HasTransactions(ctx, id)
	if err != nil {
		return false, err
	}
	if !used {
		return false, s.wallets.Delete(ctx, id)
	}
	if err := s.recurring.DeactivateForWallet(ctx, id); err != nil {
		return false, err
	}
	return true, s.wallets.Archive(ctx, id)
}
