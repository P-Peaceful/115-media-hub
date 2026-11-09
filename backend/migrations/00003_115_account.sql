-- +goose Up
CREATE TABLE provider_115_account (
 singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
 account_name TEXT NOT NULL,
 cookie TEXT NOT NULL,
 connected_at DATETIME NOT NULL
);

-- +goose Down
SELECT * FROM provider_115_down_is_irreversible_restore_a_consistent_backup;
