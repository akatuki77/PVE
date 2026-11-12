// Package store は SQLite への保存を担当する。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store は SQLite のデータベースを持つ。
type Store struct {
	db *sql.DB
}

// Open はデータベースを WAL モードで開く。ファイルやフォルダがなければ作る。
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("DBのフォルダ作成に失敗: %w", err)
	}

	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("DBを開くのに失敗: %w", err)
	}
	db.SetMaxOpenConns(1)

	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		db.Close()
		return nil, fmt.Errorf("DBの初期化に失敗: %w", err)
	}
	if mode != "wal" {
		db.Close()
		return nil, fmt.Errorf("WALモードにできません（journal_mode=%s）", mode)
	}
	return &Store{db: db}, nil
}

// PingContext はデータベースが使えるかを確かめる。
func (s *Store) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close はデータベースを閉じる。
func (s *Store) Close() error {
	return s.db.Close()
}
