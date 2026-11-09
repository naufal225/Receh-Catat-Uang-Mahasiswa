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
	plans     PlanLimits
}

// PlanLimits is the part of MembershipService that other use cases need.
type PlanLimits interface {
	Limits(ctx context.Context, userID int64) (domain.Limits, error)
}

// userLockKey gives each user a keymutex key that can never equal a wallet id
// (wallet ids are positive), for "count, then create" checks against a limit.
func userLockKey(userID int64) int64 { return -userID }

func NewWalletService(w domain.WalletRepository, r domain.RecurringRepository, locks *keymutex.Map, n domain.ChangeNotifier, plans PlanLimits) *WalletService {
	return &WalletService{wallets: w, recurring: r, locks: locks, notify: notifier{n}, plans: plans}
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
	// Count and create under one per-user lock, so two parallel requests cannot
	// both slip in under the limit.
	unlock := s.locks.Lock(userLockKey(userID))
	defer unlock()
	limits, err := s.plans.Limits(ctx, userID)
	if err != nil {
		return nil, err
	}
	if limits.MaxWallets != domain.Unlimited {
		current, err := s.wallets.List(ctx, userID, false)
		if err != nil {
			return nil, err
		}
		if !domain.Allows(limits.MaxWallets, len(current)) {
			return nil, domain.PlanLimit("Paket Gratis bisa memakai " + itoa(limits.MaxWallets) + " dompet aktif. Upgrade ke Premium untuk dompet tanpa batas, atau arsipkan dompet yang tidak dipakai.")
		}
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
