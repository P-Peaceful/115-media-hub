-- +goose Up
-- Mark the file as an application database without introducing unused business tables.
PRAGMA application_id = 115;

-- +goose Down
PRAGMA application_id = 0;
