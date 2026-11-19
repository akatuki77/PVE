package store

import (
	"context"
	"fmt"
	"time"
)

// Metric はある時点の1台分の使用量。
type Metric struct {
	VMID    int
	Time    time.Time
	CPU     float64 // 0〜1
	CPUs    int
	MemUsed int64
	MemMax  int64
}

// SaveMetrics は使用量をまとめて保存する。
func (s *Store) SaveMetrics(ctx context.Context, ms []Metric) error {
	if len(ms) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO metrics (vmid, ts, cpu, cpus, mem_used, mem_max) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("使用量の保存の準備に失敗: %w", err)
	}
	defer stmt.Close()

	for _, m := range ms {
		if _, err := stmt.ExecContext(ctx, m.VMID, m.Time.Unix(), m.CPU, m.CPUs, m.MemUsed, m.MemMax); err != nil {
			return fmt.Errorf("使用量の保存に失敗: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("使用量の保存の確定に失敗: %w", err)
	}
	return nil
}

// Metrics は指定 VM の since 以降の使用量を古い順に返す。
func (s *Store) Metrics(ctx context.Context, vmid int, since time.Time) ([]Metric, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ts, cpu, cpus, mem_used, mem_max FROM metrics WHERE vmid = ? AND ts >= ? ORDER BY ts`,
		vmid, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("使用量の取得に失敗: %w", err)
	}
	defer rows.Close()

	var ms []Metric
	for rows.Next() {
		m := Metric{VMID: vmid}
		var ts int64
		if err := rows.Scan(&ts, &m.CPU, &m.CPUs, &m.MemUsed, &m.MemMax); err != nil {
			return nil, fmt.Errorf("使用量の読み取りに失敗: %w", err)
		}
		m.Time = time.Unix(ts, 0)
		ms = append(ms, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("使用量の読み取りに失敗: %w", err)
	}
	return ms, nil
}

// DeleteMetricsBefore は before より古い使用量を消し、件数を返す。
func (s *Store) DeleteMetricsBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM metrics WHERE ts < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("古い使用量の削除に失敗: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("削除件数の取得に失敗: %w", err)
	}
	return n, nil
}
