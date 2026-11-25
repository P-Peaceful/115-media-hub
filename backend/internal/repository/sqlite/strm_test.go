// strm_test.go：验证 STRM 迁移保留归属与播放身份，以及写入意图对原内容和源身份的事务保护。

package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/strm"
	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

// TestStrmMigrationPreservesOwnershipAndPendingWrite 验证旧台账与待发布意图升级后保留身份，允许核验后补播放地址且拒绝破坏性降级。
func TestStrmMigrationPreservesOwnershipAndPendingWrite(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "version-6.db")
	pool, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	_, err = pool.ExecContext(ctx, `INSERT INTO strm_artifacts (
		id, library_id, account_id, source_file_id, playback_code, source_path, source_name,
		output_root, relative_path, target_path, content_hash, playback_token, size_bytes, generated_at
	) VALUES ('saved', 'library', 'account', 'source', 'code', '/Movie.mkv', 'Movie.mkv',
		'/media', 'Movie.strm', '/media/Movie.strm', 'old-digest', 'retained-token', 50, '2026-11-23T08:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	const pendingJSON = `{
		"ContentHash":"pending-digest", "PlaybackToken":"pending-token", "ID":"pending",
		"LibraryID":"library","AccountID":"account","SourceFileID":"second",
		"PlaybackCode":"code-2","SourcePath":"/Next.mkv","SourceName":"Next.mkv",
		"OutputRoot":"/media","RelativePath":"Next.strm","TargetPath":"/media/Next.strm",
		"SizeBytes":51,"GeneratedAt":"2026-11-23T08:01:00Z"
	}`
	if _, err := pool.ExecContext(ctx, `INSERT INTO strm_write_intents
		(target_path, library_id, artifact_json, prior_hash) VALUES ('/media/Next.strm', 'library', ?, '')`, pendingJSON); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	var hashColumns, storedDigests int
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM (
		SELECT name FROM pragma_table_info('strm_artifacts') WHERE name = 'content_hash'
		UNION ALL SELECT name FROM pragma_table_info('strm_write_intents') WHERE name = 'prior_hash'
	)`).Scan(&hashColumns); err != nil || hashColumns != 0 {
		t.Fatalf("old hash columns present (%d): %v", hashColumns, err)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM strm_write_intents WHERE json_type(artifact_json, '$.ContentHash') IS NOT NULL`).Scan(&storedDigests); err != nil || storedDigests != 0 {
		t.Fatalf("pending write still stores a hash (%d): %v", storedDigests, err)
	}
	if _, err := provider.DownTo(ctx, 6); err == nil || !strings.Contains(err.Error(), "strm_content_down_requires_consistent_backup") {
		t.Fatalf("unexpected downgrade result: %v", err)
	}
	saved, err := db.StrmArtifactByTarget(ctx, "/media/Movie.strm")
	if err != nil || saved.ID != "saved" || saved.LibraryID != "library" || saved.AccountID != "account" || saved.SourceFileID != "source" || saved.PlaybackToken != "retained-token" || saved.PlaybackURL != "" {
		t.Fatalf("existing artifact lost its ownership or token: %+v %v", saved, err)
	}
	intent, err := db.StrmIntentByTarget(ctx, "/media/Next.strm")
	if err != nil || !intent.Legacy || intent.PreviousContent != nil || intent.Artifact.ID != "pending" || intent.Artifact.SourceFileID != "second" || intent.Artifact.PlaybackToken != "pending-token" {
		t.Fatalf("existing write intent lost its identity: %+v %v", intent, err)
	}
	intent.Artifact.PlaybackURL = "https://media.test/p/pending/pending-token/Next.mkv"
	if err := db.CompleteStrmIntent(ctx, intent); err != nil {
		t.Fatalf("complete migrated intent: %v", err)
	}
	recovered, err := db.StrmArtifactByTarget(ctx, intent.Artifact.TargetPath)
	if err != nil || recovered.PlaybackURL != intent.Artifact.PlaybackURL || recovered.PlaybackToken != intent.Artifact.PlaybackToken {
		t.Fatalf("recovered artifact lost its URL or token: %+v %v", recovered, err)
	}
	if _, err := db.StrmIntentByTarget(ctx, intent.Artifact.TargetPath); !errors.Is(err, strm.ErrNotFound) {
		t.Fatalf("completed intent remains: %v", err)
	}
}

// TestStrmIntentClaimChecksRecordedOwnerAndPreviousContent 验证不同源或旧内容不匹配时无法认领或完成意图，合法覆盖保留播放令牌。
func TestStrmIntentClaimChecksRecordedOwnerAndPreviousContent(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	artifact := strm.Artifact{
		ID: "artifact-id", LibraryID: "library", AccountID: "account", SourceFileID: "source",
		TargetPath: "/media/Movie.strm", PlaybackToken: "token", GeneratedAt: time.Now(),
		PlaybackURL: "https://old.example.test/p/artifact-id/token/source",
	}
	if err := db.SaveStrmIntent(ctx, strm.WriteIntent{Artifact: artifact}); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteStrmIntent(ctx, strm.WriteIntent{Artifact: artifact}); err != nil {
		t.Fatal(err)
	}
	previousContent := artifact.PlaybackURL + "\n"
	artifact.PlaybackURL = "https://new.example.test/p/artifact-id/token/source"
	intent := strm.WriteIntent{Artifact: artifact, PreviousContent: &previousContent}
	differentOwner := intent
	differentOwner.Artifact.SourceFileID = "different-source"
	if err := db.SaveStrmIntent(ctx, differentOwner); !errors.Is(err, strm.ErrConflict) {
		t.Fatalf("claimed existing output for another source: %v", err)
	}
	differentContent := intent
	modified := strings.TrimSpace(previousContent)
	differentContent.PreviousContent = &modified
	if err := db.SaveStrmIntent(ctx, differentContent); !errors.Is(err, strm.ErrConflict) {
		t.Fatalf("claimed an output with a different previous content: %v", err)
	}
	if err := db.SaveStrmIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteStrmIntent(ctx, differentContent); !errors.Is(err, strm.ErrConflict) {
		t.Fatalf("completed a different write intent: %v", err)
	}
	if err := db.CompleteStrmIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	got, err := db.StrmArtifactByTarget(ctx, artifact.TargetPath)
	if err != nil || got.PlaybackURL != artifact.PlaybackURL || got.PlaybackToken != artifact.PlaybackToken {
		t.Fatalf("overwrite lost playback URL or token: %+v %v", got, err)
	}
}
