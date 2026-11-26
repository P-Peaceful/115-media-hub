-- +goose Up
-- 独立保存下载意图；没有媒体库外键，删除配置不能级联丢失待恢复文件身份。
CREATE TABLE sidecar_download_intents (
    id TEXT PRIMARY KEY NOT NULL CHECK (length(id) > 0),
    job_id TEXT NOT NULL CHECK (length(job_id) > 0),
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    library_id TEXT NOT NULL CHECK (length(library_id) > 0),
    account_id TEXT NOT NULL CHECK (length(account_id) > 0),
    source_file_id TEXT NOT NULL CHECK (length(source_file_id) > 0),
    source_directory_id TEXT NOT NULL CHECK (length(source_directory_id) > 0),
    kind TEXT NOT NULL CHECK (kind IN ('nfo', 'image')),
    output_root TEXT NOT NULL CHECK (length(output_root) > 0),
    relative_path TEXT NOT NULL CHECK (length(relative_path) > 0),
    target_path TEXT NOT NULL CHECK (length(target_path) > 0),
    temporary_name TEXT NOT NULL DEFAULT '',
    temporary_device TEXT NOT NULL DEFAULT '',
    temporary_inode TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('reserved', 'staging', 'staged', 'published')),
    expected_bytes INTEGER CHECK (expected_bytes >= 0),
    completed_bytes INTEGER NOT NULL DEFAULT 0 CHECK (completed_bytes >= 0),
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT sidecar_intents_target_unique UNIQUE (target_path),
    CONSTRAINT sidecar_intents_size_bound CHECK (
        completed_bytes <= CASE kind WHEN 'nfo' THEN 8388608 ELSE 67108864 END
        AND (expected_bytes IS NULL OR (expected_bytes <= CASE kind WHEN 'nfo' THEN 8388608 ELSE 67108864 END
            AND completed_bytes <= expected_bytes))
    ),
    CONSTRAINT sidecar_intents_phase_identity CHECK (
        (state = 'reserved' AND temporary_device = '' AND temporary_inode = '' AND completed_bytes = 0)
        OR (state = 'staging' AND length(temporary_name) > 0 AND length(temporary_device) > 0
            AND length(temporary_inode) > 0 AND completed_bytes = 0)
        OR (state IN ('staged', 'published') AND length(temporary_name) > 0 AND length(temporary_device) > 0
            AND length(temporary_inode) > 0 AND (expected_bytes IS NULL OR completed_bytes = expected_bytes))
    )
);
CREATE INDEX sidecar_intents_library_target ON sidecar_download_intents (library_id, target_path);

-- +goose Down
-- 未完成的文件动作需要保留身份；非空时拒绝回退，不能靠删表丢失恢复线索。
CREATE TEMP TABLE sidecar_intents_down_guard (
    occupied INTEGER CONSTRAINT sidecar_intents_down_requires_consistent_backup CHECK (occupied = 0)
);
INSERT INTO sidecar_intents_down_guard SELECT EXISTS (SELECT 1 FROM sidecar_download_intents);
DROP TABLE sidecar_intents_down_guard;
DROP TABLE sidecar_download_intents;
