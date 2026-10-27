-- +goose Up
ALTER TABLE connections ADD COLUMN profile_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE connections DROP COLUMN profile_json;
