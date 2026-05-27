package wallet

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"net/http"
)

type Service struct {
	walletRepo Repository
	logger     *logger.Logger
}

func NewService(walletRepo Repository, logger *logger.Logger) *Service {
	return &Service{
		walletRepo: walletRepo,
		logger:     logger,
	}
}

func (s *Service) IsReady(ctx context.Context) error {
	const op richerror.Operation = "Service.IsReady"

	wErr := s.walletRepo.Ping(ctx)
	if wErr != nil {
		return richerror.New(op).
			WithWrapper(wErr).
			WithMessage("db down").
			WithKind(richerror.KindUnavailable)
	}

	return nil
}

func (s *Service) Transfer(ctx context.Context, userId uint64, req TransferRequest) (*TransferResponse, error, int) {
	const op richerror.Operation = "Service.Transfer"

	operationId := uuid.New().String()
	err := s.walletRepo.RunInTx(ctx, func(tx TxOps) error {
		sourceBalance, err := tx.GetBalanceForUpdate(ctx, req.SourceId)
		if err != nil {
			// log error
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrTransferFailed).
				WithKind(richerror.KindInternal)
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
