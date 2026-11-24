package strm

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerationCreatesMissingOutputDirectoriesAfterReadOnlyPreview(t *testing.T) {
	service, repo, execution, _ := newGenerationFixture(t, singleVideoProvider(), io.Discard)
	parent := filepath.Join(execution.Library.OutputPath, "新目录")
	execution.Library.OutputPath = filepath.Join(parent, "媒体")
	if got := inspectOutputRoot(execution.Library.OutputPath); got.Status != "will_create" {
		t.Fatalf("unexpected preview: %+v", got)
	}
	if _, err := os.Stat(parent); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("preview created or could not inspect parent directory: %v", err)
	}
	requireGeneration(t, service, execution)
	path := filepath.Join(execution.Library.OutputPath, "样片 1.strm")
	if content := requireFile(t, path); !strings.HasPrefix(content, "https://media.example.test/p/") {
		t.Fatal("sync did not generate a playback URL in the new output directory")
	}
	if len(repo.artifacts) != 1 || len(repo.intents) != 0 {
		t.Fatal("generated artifact was not registered")
	}
	if got := inspectOutputRoot(execution.Library.OutputPath); got.Status != "ready" {
		t.Fatalf("unexpected preview after generation: %+v", got)
	}
}

func TestOutputRootKeepsExistingAliasesAndRejectsBrokenLinks(t *testing.T) {
	parent := t.TempDir()
	alias := filepath.Join(parent, "alias")
	real := t.TempDir()
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if got := inspectOutputRoot(alias); got.Status != "ready" {
		t.Fatalf("existing alias unavailable: %+v", got)
	}
	root, canonical, err := prepareOutputRoot(t.Context(), filepath.Join(alias, "child"))
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(real, "child"))
	if err != nil || canonical != want {
		t.Fatalf("did not use the canonical output path: %q (%v)", canonical, err)
	}

	broken := filepath.Join(parent, "broken")
	missing := filepath.Join(real, "missing")
	if err := os.Symlink(missing, broken); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{broken + "/", filepath.Join(broken, "child")} {
		if got := inspectOutputRoot(path); got.Status != "unavailable" {
			t.Fatalf("broken link was treated as a creatable directory: %+v", got)
		}
		if _, _, err := prepareOutputRoot(t.Context(), path); !errors.Is(err, ErrConflict) {
			t.Fatalf("broken link was accepted: %v", err)
		}
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("broken link target was created: %v", err)
	}
}

func TestGenerationReportsOutputPathCollisionWithoutLoggingPath(t *testing.T) {
	var logs bytes.Buffer
	service, _, execution, result := newGenerationFixture(t, singleVideoProvider(), &logs)
	path := filepath.Join(execution.Library.OutputPath, "private-output-file")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	execution.Library.OutputPath = filepath.Join(path, "child")
	preview := inspectOutputRoot(execution.Library.OutputPath)
	if preview.Status != "not_directory" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	err := service.Module().Run(t.Context(), execution)
	code, message := generationError(err)
	if code != "strm_output_root_not_directory" || message != preview.Message || len(result.logs) != 1 || result.logs[0] != message {
		t.Fatalf("failure not categorized for users and logs: %q, %q, %v", code, message, result.logs)
	}
	if !strings.Contains(logs.String(), code) || strings.Contains(logs.String(), path) || requireFile(t, path) != "keep me" {
		t.Fatal("missing stable error code, leaked path or modified existing file")
	}
}

func TestOutputPreviewChecksEffectiveWriteAccess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to the test directory")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{parent, filepath.Join(parent, "missing")} {
		if got := inspectOutputRoot(path); got.Status != "permission_denied" {
			t.Fatalf("non-writable output path was accepted: %+v", got)
		}
		if _, _, err := prepareOutputRoot(t.Context(), path); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("expected actionable output permission failure, got %v", err)
		}
	}
}
