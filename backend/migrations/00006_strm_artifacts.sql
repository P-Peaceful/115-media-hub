-- +goose Up
CREATE TABLE strm_artifacts (
    id TEXT PRIMARY KEY NOT NULL,
    library_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    source_file_id TEXT NOT NULL,
    playback_code TEXT NOT NULL,
    source_path TEXT NOT NULL,
    source_name TEXT NOT NULL,
    output_root TEXT NOT NULL,
    relative_path TEXT NOT NULL,
    target_path TEXT NOT NULL UNIQUE,
    content_hash TEXT NOT NULL,
    playback_token TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    generated_at DATETIME NOT NULL
);

CREATE INDEX strm_artifacts_library_generated ON strm_artifacts (library_id, generated_at, id);
CREATE INDEX strm_artifacts_generated ON strm_artifacts (generated_at, id);

CREATE TABLE strm_write_intents (
    target_path TEXT PRIMARY KEY NOT NULL,
    library_id TEXT NOT NULL,
    artifact_json TEXT NOT NULL,
    prior_hash TEXT NOT NULL
);

CREATE INDEX strm_write_intents_library ON strm_write_intents (library_id, target_path);

-- +goose Down
-- 有文件归属或待恢复意图时要求还原一致性备份。
CREATE TEMP TABLE strm_down_guard (
    occupied INTEGER CONSTRAINT strm_down_requires_consistent_backup CHECK (occupied = 0)
);
INSERT INTO strm_down_guard
SELECT EXISTS (SELECT 1 FROM strm_artifacts) OR EXISTS (SELECT 1 FROM strm_write_intents);
DROP TABLE strm_down_guard;
DROP TABLE strm_write_intents;
DROP TABLE strm_artifacts;
