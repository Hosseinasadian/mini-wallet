package repository

import (
	"context"
	wallet "github.com/hosseinasadian/mini-wallet/internal/wallet/service"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db     *sqlx.DB
	logger *pkgLogger.Logger
}

func NewRepository(db *sqlx.DB, logger *pkgLogger.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

func (repo *Repository) Ping(ctx context.Context) error {
	return repo.db.PingContext(ctx)
}

func (repo *Repository) GetBalance(ctx context.Context, walletID uint64) (int64, error) {
	var balance int64
	err := repo.db.GetContext(ctx, &balance, "SELECT balance FROM wallets WHERE id = ?", walletID)
	if err != nil {
		return balance, err
	}
	return balance, nil
}

func (repo *Repository) RunInTx(ctx context.Context, fn func(exec wallet.TxOps) error) error {
	var err error
	var tx *sqlx.Tx
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer func(err error) {
		if err != nil {
			_ = tx.Rollback()
		}
	}(err)

	if err = fn(&txRepository{tx: tx}); err != nil {
		return err
	}

	return tx.Commit()

}

type txRepository struct {
	tx *sqlx.Tx
}

func (repo *txRepository) GetBalanceForUpdate(ctx context.Context, walletID uint64) (int64, error) {
	var balance int64
	err := repo.tx.GetContext(ctx, &balance, "SELECT balance FROM wallets WHERE id = ? FOR UPDATE", walletID)
	if err != nil {
		return balance, err
	}
	return balance, nil
}

func (repo *txRepository) IncreaseBalance(ctx context.Context, walletID uint64, amount int64) error {
	_, err := repo.tx.ExecContext(ctx, "UPDATE wallets SET balance = balance + ? WHERE id = ?", amount, walletID)
	return err
}

func (repo *txRepository) DecreaseBalance(ctx context.Context, walletID uint64, amount int64) error {
	_, err := repo.tx.ExecContext(ctx, "UPDATE wallets SET balance = balance - ? WHERE id = ?", amount, walletID)
	return err
}

func (repo *txRepository) CreateTransaction(ctx context.Context, req wallet.CreateTransactionRequest) (uint64, error) {
	result, err := repo.tx.ExecContext(ctx, `
		INSERT INTO wallet_transactions (wallet_id, operation_id, type, amount, reference_id, description, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.WalletID,
		req.OperationID,
		req.Type,
		req.Amount,
		req.ReferenceID,
		req.Description,
		req.Status,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return uint64(id), nil
}
