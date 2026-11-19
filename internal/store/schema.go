package store

// schema はテーブル定義。起動のたびに実行する。
const schema = `
CREATE TABLE IF NOT EXISTS metrics (
	vmid     INTEGER NOT NULL,
	ts       INTEGER NOT NULL, -- UNIX時刻（秒）
	cpu      REAL    NOT NULL, -- 0〜1
	cpus     INTEGER NOT NULL,
	mem_used INTEGER NOT NULL, -- バイト
	mem_max  INTEGER NOT NULL  -- バイト
);
CREATE INDEX IF NOT EXISTS metrics_vmid_ts ON metrics (vmid, ts);
`
