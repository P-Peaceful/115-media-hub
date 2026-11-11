-- +goose Up
CREATE TABLE media_libraries (
 id TEXT PRIMARY KEY NOT NULL,
 config_json TEXT NOT NULL,
 revision INTEGER NOT NULL DEFAULT 1,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);

-- +goose Down
-- Restore a consistent backup to recover deleted configurations.
SELECT * FROM media_libraries_down_is_irreversible_restore_a_consistent_backup;
