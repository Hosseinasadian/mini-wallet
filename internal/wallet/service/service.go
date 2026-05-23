package wallet

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/http"
)

type Service struct {
	walletRepo Repository
}

func NewService(walletRepo Repository) *Service {
	return &Service{
		walletRepo: walletRepo,
	}
}

func (s *Service) IsReady(ctx context.Context) (error, int) {
	wErr := s.walletRepo.Ping(ctx)
	if wErr != nil {
		return errors.New("wallet db down"), http.StatusServiceUnavailable
	}

	return nil, http.StatusOK
}

func (s *Service) Transfer(ctx context.Context, userId uint64, req TransferRequest) (*TransferResponse, error, int) {
	operationId := uuid.New().String()
	err := s.walletRepo.RunInTx(ctx, func(tx TxOps) error {
		sourceBalance, err := tx.GetBalanceForUpdate(ctx, req.SourceId)
		if err != nil {
			return err
		}

		if sourceBalance < req.Amount {
			return fmt.Errorf("source balance is too low")
		}

		err = tx.DecreaseBalance(ctx, req.SourceId, req.Amount)
		if err != nil {
			return err
		}

		_, err = tx.CreateTransaction(ctx, CreateTransactionRequest{
			WalletID:    req.SourceId,
			OperationID: operationId,
			Amount:      req.Amount,
			ReferenceID: uuid.New().String(),
			Type:        TransactionTypeTransferOut,
			Status:      TransactionStatusCompleted,
		})
		if err != nil {
			return err
		}

		err = tx.IncreaseBalance(ctx, req.DestId, req.Amount)
		if err != nil {
			return err
		}

		_, err = tx.CreateTransaction(ctx, CreateTransactionRequest{
			WalletID:    req.DestId,
			OperationID: operationId,
			Amount:      req.Amount,
			ReferenceID: uuid.New().String(),
			Type:        TransactionTypeTransferIn,
			Status:      TransactionStatusCompleted,
		})
		if err != nil {
			return err
		}

		return nil

	})
	if err != nil {
		return nil, err, http.StatusInternalServerError
	}
	return &TransferResponse{
		Message:     "Transaction successfully transferred",
		OperationID: &operationId,
	}, nil, http.StatusOK
}
