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

CREATE TABLE IF NOT EXISTS autoscale_targets (
	vmid       INTEGER PRIMARY KEY,
	name       TEXT    NOT NULL, -- 選んだときの VM 名
	created_at INTEGER NOT NULL  -- UNIX時刻（秒）
);

CREATE TABLE IF NOT EXISTS events (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ts      INTEGER NOT NULL, -- UNIX時刻（秒）
	vmid    INTEGER NOT NULL,
	kind    TEXT    NOT NULL, -- "autoscale" など
	message TEXT    NOT NULL,
	dry_run INTEGER NOT NULL  -- 1 なら実際には変更していない
);
`
