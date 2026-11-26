package store

import (
	"context"
	"fmt"
	"time"
)

// Event はオートスケールなどで行ったこと（行う予定のこと）の記録。
type Event struct {
	ID      int64     `json:"id"`
	Time    time.Time `json:"time"`
	VMID    int       `json:"vmid"`
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
	DryRun  bool      `json:"dry_run"`
}

// AddEvent は記録を保存し、ID を入れて返す。
func (s *Store) AddEvent(ctx context.Context, e Event) (Event, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events (ts, vmid, kind, message, dry_run) VALUES (?, ?, ?, ?, ?)`,
		e.Time.Unix(), e.VMID, e.Kind, e.Message, e.DryRun)
	if err != nil {
		return Event{}, fmt.Errorf("記録の保存に失敗: %w", err)
	}
	if e.ID, err = res.LastInsertId(); err != nil {
		return Event{}, fmt.Errorf("記録のIDの取得に失敗: %w", err)
	}
	return e, nil
}

// Events は新しい順に最大 limit 件返す。
func (s *Store) Events(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, vmid, kind, message, dry_run FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("記録の取得に失敗: %w", err)
	}
	defer rows.Close()

	es := []Event{}
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.VMID, &e.Kind, &e.Message, &e.DryRun); err != nil {
			return nil, fmt.Errorf("記録の読み取りに失敗: %w", err)
		}
		e.Time = time.Unix(ts, 0)
		es = append(es, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("記録の読み取りに失敗: %w", err)
	}
	return es, nil
}

// DeleteEventsBefore は before より古い記録を消し、件数を返す。
func (s *Store) DeleteEventsBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE ts < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("古い記録の削除に失敗: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("削除件数の取得に失敗: %w", err)
	}
	return n, nil
}
