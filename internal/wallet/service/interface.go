package wallet

import "context"

type TxOps interface {
	GetBalanceForUpdate(ctx context.Context, walletID uint64) (int64, error)
	IncreaseBalance(ctx context.Context, walletID uint64, amount int64) error
	DecreaseBalance(ctx context.Context, walletID uint64, amount int64) error
	CreateTransaction(ctx context.Context, req CreateTransactionRequest) (uint64, error)
}

type Repository interface {
	RunInTx(ctx context.Context, fn func(exec TxOps) error) error
	Ping(ctx context.Context) error
}
