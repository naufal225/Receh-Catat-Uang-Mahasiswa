package domain

import (
	"context"
	"time"
)

// TxSuggestion is a name the user wrote before, with what they used last time,
// so typing "Mak…" can fill in category, wallet and amount.
type TxSuggestion struct {
	Name       string `json:"name"`
	Type       TxType `json:"type"`
	CategoryID int64  `json:"categoryId"`
	WalletID   int64  `json:"walletId"`
	Amount     int64  `json:"amount"`
	Uses       int    `json:"uses"`
}

// ImportBatch groups the transactions of one imported statement, so the whole
// import can be undone in one step.
type ImportBatch struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"-"`
	WalletID   int64     `json:"walletId"`
	WalletName string    `json:"walletName"`
	FileName   string    `json:"fileName"`
	Rows       int       `json:"rows"`
	TotalIn    int64     `json:"totalIn"`
	TotalOut   int64     `json:"totalOut"`
	Remaining  int       `json:"remaining"` // transactions of the batch still present
	CreatedAt  time.Time `json:"createdAt"`
}

// DuplicateKey identifies a transaction for duplicate detection on import.
func DuplicateKey(date string, t TxType, amount int64, name string) string {
	return date + "|" + string(t) + "|" + itoa64(amount) + "|" + lowerTrim(name)
}

type EntryRepository interface {
	// Suggest returns past names starting with prefix (or equal to it when
	// exact), most used first, each with the details of its latest use.
	Suggest(ctx context.Context, userID int64, prefix string, exact bool, limit int) ([]TxSuggestion, error)
	// ExistingKeys returns DuplicateKeys of a wallet's transactions in a range.
	ExistingKeys(ctx context.Context, userID, walletID int64, from, to string) (map[string]bool, error)
	// CreateBatch inserts the batch and all its transactions in one commit; the
	// wallet balance moves by the net amount (ErrInsufficient if it would go below zero).
	CreateBatch(ctx context.Context, b *ImportBatch, txs []Transaction) error
	Batches(ctx context.Context, userID int64) ([]ImportBatch, error)
	// UndoBatch removes the batch's remaining transactions and reverses their effect.
	UndoBatch(ctx context.Context, userID, batchID int64) error
}
