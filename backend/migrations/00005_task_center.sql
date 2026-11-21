-- +goose Up
CREATE TABLE task_schedules (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('scan', 'strm', 'sync', 'check', 'cleanup', 'scrape', 'upload')),
    library_id TEXT,
    enabled BOOLEAN NOT NULL CHECK (enabled IN (0, 1)),
    period TEXT NOT NULL CHECK (period IN ('daily', 'weekly', 'monthly', 'custom')),
    timezone TEXT NOT NULL,
    time TEXT,
    weekdays TEXT NOT NULL DEFAULT '[]',
    month_day INTEGER,
    cron TEXT,
    notify_on TEXT NOT NULL DEFAULT '[]',
    next_run_at DATETIME,
    last_skip_reason TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CHECK (enabled OR next_run_at IS NULL)
);
CREATE INDEX task_schedules_due ON task_schedules (next_run_at, id) WHERE enabled;

-- Library and schedule IDs are snapshots so removing configuration preserves history.
CREATE TABLE task_jobs (
    id TEXT PRIMARY KEY NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('scan', 'strm', 'sync', 'check', 'cleanup', 'scrape', 'upload')),
    library_id TEXT NOT NULL,
    library_name TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('manual', 'schedule', 'retry')),
    schedule_id TEXT,
    scheduled_for DATETIME,
    parent_id TEXT REFERENCES task_jobs (id),
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'paused', 'completed', 'failed', 'canceled')),
    outcome TEXT CHECK (outcome IN ('success', 'partial_failure', 'failure')),
    created_at DATETIME NOT NULL,
    started_at DATETIME,
    finished_at DATETIME,
    progress_percent REAL CHECK (progress_percent BETWEEN 0 AND 100),
    speed_bytes_per_second INTEGER CHECK (speed_bytes_per_second >= 0),
    current_file TEXT,
    total_items INTEGER CHECK (total_items >= 0),
    succeeded_items INTEGER NOT NULL DEFAULT 0 CHECK (succeeded_items >= 0),
    failed_items INTEGER NOT NULL DEFAULT 0 CHECK (failed_items >= 0),
    skipped_items INTEGER NOT NULL DEFAULT 0 CHECK (skipped_items >= 0),
    request_id TEXT NOT NULL DEFAULT '',
    pending_action TEXT NOT NULL DEFAULT '' CHECK (pending_action IN ('', 'pause', 'cancel')),
    needs_resume BOOLEAN NOT NULL DEFAULT 0 CHECK (needs_resume IN (0, 1)),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    retry_scope TEXT NOT NULL DEFAULT '' CHECK (retry_scope IN ('', 'whole', 'failed')),
    notify_on TEXT NOT NULL DEFAULT '[]',
    CONSTRAINT task_jobs_occurrence UNIQUE (schedule_id, scheduled_for, library_id),
    CHECK (status = 'running' OR pending_action = ''),
    CHECK ((status IN ('completed', 'failed', 'canceled')) = (finished_at IS NOT NULL)),
    CHECK ((status IN ('completed', 'failed')) = (outcome IS NOT NULL))
);
CREATE INDEX task_jobs_history ON task_jobs (created_at DESC, id DESC);
CREATE INDEX task_jobs_library_history ON task_jobs (library_id, created_at DESC, id DESC);
CREATE INDEX task_jobs_queue ON task_jobs (created_at, id) WHERE status = 'queued';
CREATE UNIQUE INDEX task_jobs_running_library ON task_jobs (library_id) WHERE status = 'running';
CREATE UNIQUE INDEX task_jobs_active_retry ON task_jobs (parent_id) WHERE parent_id IS NOT NULL AND status IN ('queued', 'running', 'paused');

CREATE TABLE task_items (
    id TEXT PRIMARY KEY NOT NULL,
    job_id TEXT NOT NULL REFERENCES task_jobs (id) ON DELETE CASCADE,
    item_key TEXT NOT NULL,
    path TEXT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'skipped')),
    CONSTRAINT task_items_key UNIQUE (job_id, item_key)
);
CREATE INDEX task_items_failed ON task_items (job_id, id) WHERE status = 'failed';

CREATE TABLE task_logs (
    id TEXT PRIMARY KEY NOT NULL,
    job_id TEXT NOT NULL REFERENCES task_jobs (id) ON DELETE CASCADE,
    level TEXT NOT NULL CHECK (level IN ('info', 'warn', 'error')),
    message TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX task_logs_job_time ON task_logs (job_id, created_at, id);

-- +goose Down
-- Task history cannot be restored after dropping these tables.
SELECT * FROM task_center_down_is_irreversible_restore_a_consistent_backup;
