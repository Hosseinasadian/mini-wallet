package database

import (
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/jmoiron/sqlx"
)

type MysqlLogger struct {
	logger *pkgLogger.Logger
}

func (l *MysqlLogger) Print(v ...any) {
	l.logger.Debug("mysql driver", "msg", fmt.Sprint(v...))
}

type Database struct {
	DB *sqlx.DB
}

func Connect(cfg *config.MySQL) (*Database, error) {
	dsn := fmt.Sprintf("%s:%s@(%s:%d)/%s?parseTime=true", cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, err
	}

	return &Database{DB: db}, nil
}

func Close(conn *sqlx.DB) error {
	return conn.Close()
}

func SetLogger(logger *pkgLogger.Logger) error {
	return mysql.SetLogger(&MysqlLogger{logger: logger})
}
