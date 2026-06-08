package repository

import (
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
