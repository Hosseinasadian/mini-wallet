package wallet

import (
	"time"
)

type Wallet struct {
	ID        uint64    `db:"id"`
	UserID    uint64    `db:"user_id"`
	Balance   int64     `db:"balance"`
	Currency  string    `db:"currency"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type TransactionType string

const (
	TransactionTypeDeposit     TransactionType = "deposit"
	TransactionTypeWithdraw    TransactionType = "withdraw"
	TransactionTypeFee         TransactionType = "fee"
	TransactionTypeTransferIn  TransactionType = "transfer_in"
	TransactionTypeTransferOut TransactionType = "transfer_out"
)

type TransactionStatus string

const (
	TransactionStatusPending   TransactionStatus = "pending"
	TransactionStatusCompleted TransactionStatus = "completed"
	TransactionStatusFailed    TransactionStatus = "failed"
)

type Transaction struct {
	ID          uint64            `db:"id"`
	WalletID    uint64            `db:"wallet_id"`
	OperationID string            `db:"operation_id"`
	Type        TransactionType   `db:"type"`
	Amount      int64             `db:"amount"`
	ReferenceID string            `db:"reference_id"`
	Description string            `db:"description"`
	Status      TransactionStatus `db:"status"`
	CreatedAt   time.Time         `db:"created_at"`
}
