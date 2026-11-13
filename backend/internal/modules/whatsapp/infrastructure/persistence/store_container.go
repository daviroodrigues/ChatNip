package persistence

import (
	"context"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func NewWhatsMeowContainer(dbPath string) (*sqlstore.Container, error) {
	dbLog := waLog.Stdout("Database", "WARN", true)

	dsn := "file:" + dbPath + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"

	return sqlstore.New(context.Background(), "sqlite3", dsn, dbLog)
}
