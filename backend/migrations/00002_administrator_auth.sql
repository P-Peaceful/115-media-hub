-- +goose Up
CREATE TABLE administrators (
    singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
    id TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE TABLE system_settings (
    singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
    service_url TEXT NOT NULL,
    output_directory TEXT NOT NULL,
    session_lifetime_days INTEGER NOT NULL CHECK (session_lifetime_days IN (1, 7, 30)),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE TABLE admin_sessions (
    token_hash TEXT NOT NULL PRIMARY KEY,
    administrator_id TEXT NOT NULL REFERENCES administrators(id) ON DELETE CASCADE,
    csrf_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL
);
CREATE INDEX idx_admin_sessions_expiry ON admin_sessions(expires_at);
CREATE INDEX idx_admin_sessions_administrator ON admin_sessions(administrator_id);
CREATE TABLE audit_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    target_id TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('succeeded', 'failed')),
    correlation_id TEXT NOT NULL,
    occurred_at DATETIME NOT NULL
);
CREATE INDEX idx_audit_records_occurred ON audit_records(occurred_at);

-- +goose Down
-- Credential data cannot be restored by a destructive schema downgrade.
-- Deliberately fail; restore the matching binary and a consistent pre-upgrade backup instead.
SELECT * FROM auth_down_is_irreversible_restore_a_consistent_backup;
