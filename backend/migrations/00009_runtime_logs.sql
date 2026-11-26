-- +goose Up
CREATE TABLE runtime_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    occurred_at TEXT NOT NULL,
    level TEXT NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error')),
    module TEXT NOT NULL,
    operation TEXT NOT NULL DEFAULT '',
    job_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_runtime_logs_time_id ON runtime_logs (occurred_at DESC, id DESC);
CREATE INDEX idx_runtime_logs_level_time ON runtime_logs (level, occurred_at DESC, id DESC);
CREATE INDEX idx_runtime_logs_module_time ON runtime_logs (module, occurred_at DESC, id DESC);
CREATE INDEX idx_runtime_logs_job_time ON runtime_logs (job_id, occurred_at DESC, id DESC) WHERE job_id != '';

-- +goose Down
-- Dropping a nonempty runtime log table would permanently destroy history.
CREATE TEMP TABLE runtime_logs_down_guard (
    occupied INTEGER NOT NULL CONSTRAINT runtime_logs_down_requires_consistent_backup CHECK (occupied = 0)
);
INSERT INTO runtime_logs_down_guard SELECT EXISTS (SELECT 1 FROM runtime_logs);
DROP TABLE runtime_logs_down_guard;
DROP TABLE runtime_logs;
