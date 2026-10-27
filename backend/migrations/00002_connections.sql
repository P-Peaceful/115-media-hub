-- +goose Up
CREATE TABLE connections (
    id TEXT PRIMARY KEY NOT NULL,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    external_account_id TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    credential_json TEXT NOT NULL,
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    is_active INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL,
    retry_after_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX idx_connections_one_active_115
    ON connections(kind) WHERE kind = '115' AND is_active = 1;

-- +goose Down
DROP INDEX idx_connections_one_active_115;
DROP TABLE connections;
