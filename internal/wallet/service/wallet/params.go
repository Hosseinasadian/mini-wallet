package wallet

type CreateTransactionRequest struct {
	WalletID    uint64
	OperationID string
	Type        TransactionType
	Amount      int64
	ReferenceID string
	Description string
	Status      TransactionStatus
}

type TransferRequest struct {
	SourceId uint64 `json:"source_id"`
	DestId   uint64 `json:"dest_id"`
	Amount   int64  `json:"amount"`
}

type TransferResponse struct {
	Message     string  `json:"message"`
	OperationID *string `json:"operation_id"`
}

type CreateWalletRequest struct {
	UserID   uint64 `db:"user_id"`
	UUID     string `db:"uuid"`
	Balance  int64  `db:"balance"`
	Currency string `db:"currency"`
}
