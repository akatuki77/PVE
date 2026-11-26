package store

import (
	"context"
	"fmt"
	"time"
)

// Target はオートスケールの対象に選んだ VM。
type Target struct {
	VMID      int
	Name      string // 選んだときの VM 名
	CreatedAt time.Time
}

// Targets は対象の VM を VMID 順に返す。
func (s *Store) Targets(ctx context.Context) ([]Target, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT vmid, name, created_at FROM autoscale_targets ORDER BY vmid`)
	if err != nil {
		return nil, fmt.Errorf("対象VMの取得に失敗: %w", err)
	}
	defer rows.Close()

	var ts []Target
	for rows.Next() {
		var t Target
		var created int64
		if err := rows.Scan(&t.VMID, &t.Name, &created); err != nil {
			return nil, fmt.Errorf("対象VMの読み取りに失敗: %w", err)
		}
		t.CreatedAt = time.Unix(created, 0)
		ts = append(ts, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("対象VMの読み取りに失敗: %w", err)
	}
	return ts, nil
}

// AddTarget は VM を対象にする。すでにあれば名前を更新する。
func (s *Store) AddTarget(ctx context.Context, vmid int, name string, now time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO autoscale_targets (vmid, name, created_at) VALUES (?, ?, ?)
		 ON CONFLICT (vmid) DO UPDATE SET name = excluded.name, created_at = excluded.created_at`,
		vmid, name, now.Unix())
	if err != nil {
		return fmt.Errorf("対象VMの追加に失敗: %w", err)
	}
	return nil
}

// RemoveTarget は VM を対象から外す。なくてもエラーにしない。
func (s *Store) RemoveTarget(ctx context.Context, vmid int) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM autoscale_targets WHERE vmid = ?`, vmid); err != nil {
		return fmt.Errorf("対象VMの削除に失敗: %w", err)
	}
	return nil
}
