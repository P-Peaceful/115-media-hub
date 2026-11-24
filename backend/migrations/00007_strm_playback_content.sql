-- +goose Up
ALTER TABLE strm_artifacts ADD COLUMN playback_url TEXT NOT NULL DEFAULT '';
ALTER TABLE strm_artifacts DROP COLUMN content_hash;

ALTER TABLE strm_write_intents ADD COLUMN previous_content TEXT;
ALTER TABLE strm_write_intents ADD COLUMN legacy INTEGER NOT NULL DEFAULT 0 CHECK (legacy IN (0, 1));
UPDATE strm_write_intents SET artifact_json = json_remove(artifact_json, '$.ContentHash'), legacy = 1;
ALTER TABLE strm_write_intents DROP COLUMN prior_hash;

-- +goose Down
-- 数据库有 STRM 登记时，恢复旧版本需要包含原有摘要的数据库备份。
CREATE TEMP TABLE strm_content_down_guard (
    occupied INTEGER CONSTRAINT strm_content_down_requires_consistent_backup CHECK (occupied = 0)
);
INSERT INTO strm_content_down_guard
SELECT EXISTS (SELECT 1 FROM strm_artifacts) OR EXISTS (SELECT 1 FROM strm_write_intents);
DROP TABLE strm_content_down_guard;

ALTER TABLE strm_write_intents ADD COLUMN prior_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE strm_write_intents DROP COLUMN legacy;
ALTER TABLE strm_write_intents DROP COLUMN previous_content;

ALTER TABLE strm_artifacts ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE strm_artifacts DROP COLUMN playback_url;
