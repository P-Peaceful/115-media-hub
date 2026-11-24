package strm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

type generationRepository struct {
	Repository
	artifacts    map[string]Artifact
	intents      map[string]WriteIntent
	completeErr  error
	beforeIntent func() error
}

func (r *generationRepository) StrmArtifactByTarget(_ context.Context, target string) (Artifact, error) {
	item, found := r.artifacts[target]
	if !found {
		return Artifact{}, ErrNotFound
	}
	return item, nil
}

func (r *generationRepository) StrmIntentByTarget(_ context.Context, target string) (WriteIntent, error) {
	intent, found := r.intents[target]
	if !found {
		return WriteIntent{}, ErrNotFound
	}
	return intent, nil
}

func (r *generationRepository) SaveStrmIntent(_ context.Context, intent WriteIntent) error {
	if r.beforeIntent != nil {
		if err := r.beforeIntent(); err != nil {
			return err
		}
	}
	r.intents[intent.Artifact.TargetPath] = intent
	return nil
}

func (r *generationRepository) CompleteStrmIntent(_ context.Context, intent WriteIntent) error {
	if r.completeErr != nil {
		return r.completeErr
	}
	r.artifacts[intent.Artifact.TargetPath] = intent.Artifact
	delete(r.intents, intent.Artifact.TargetPath)
	return nil
}

func (r *generationRepository) DiscardStrmIntent(_ context.Context, target string) error {
	delete(r.intents, target)
	return nil
}

func (r *generationRepository) StrmIntentsByLibrary(_ context.Context, libraryID string) ([]WriteIntent, error) {
	var result []WriteIntent
	for _, intent := range r.intents {
		if intent.Artifact.LibraryID == libraryID {
			result = append(result, intent)
		}
	}
	return result, nil
}

type generationProvider struct {
	Provider
	list func(context.Context, provider115.FileQuery) (provider115.FilePage, error)
}

func (p generationProvider) Files(ctx context.Context, query provider115.FileQuery, _ string) (provider115.FilePage, error) {
	return p.list(ctx, query)
}

type generationResult struct {
	items    []domain.Item
	progress domain.Progress
	logs     []string
}

func newGenerationFixture(t *testing.T, provider generationProvider, logOutput io.Writer) (*Service, *generationRepository, jobs.Execution, *generationResult) {
	t.Helper()
	repo := &generationRepository{artifacts: make(map[string]Artifact), intents: make(map[string]WriteIntent)}
	service := New(repo, provider, slog.New(slog.NewJSONHandler(logOutput, nil)))
	result := &generationResult{}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := library.Library{ID: "lib-test", Config: library.Config{
		DirectoryID: "42", DirectoryPath: "115 网盘/媒体", OutputPath: root, Enabled: true,
		GenerateSTRM: true, LinkType: "proxy", STRMLinkFormat: "original_name", STRMExistingFilePolicy: "skip",
		STRMDirectDomain: "media.example.test",
	}}
	execution := jobs.Execution{
		Job: domain.Job{ID: "task-test", Attempt: 1, RequestID: "request-test"}, Library: target,
		Report: func(_ context.Context, value domain.Progress) error { result.progress = value; return nil },
		Item:   func(_ context.Context, item domain.Item) error { result.items = append(result.items, item); return nil },
		Log: func(_ context.Context, _, message string) error {
			result.logs = append(result.logs, message)
			return nil
		},
	}
	return service, repo, execution, result
}

func rootPage(items ...provider115.FileItem) provider115.FilePage {
	return provider115.FilePage{
		DirectoryID: "42", AccountID: "private-account", Items: items,
		Path: []provider115.FilePath{{ID: "0", Name: "115 网盘"}, {ID: "42", Name: "媒体"}},
	}
}

func videoFile(id, name string) provider115.FileItem {
	return provider115.FileItem{ID: id, Name: name, FileType: "video", PlaybackCode: "private-playback-code"}
}

func singleVideoProvider() generationProvider {
	return generationProvider{list: func(context.Context, provider115.FileQuery) (provider115.FilePage, error) {
		return rootPage(videoFile("123", "样片 1.mkv")), nil
	}}
}

func requireGeneration(t *testing.T, service *Service, execution jobs.Execution) {
	t.Helper()
	if err := service.Module().Run(t.Context(), execution); err != nil {
		t.Fatal(err)
	}
}

func requireFile(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestGenerationScansPagesAndProcessesIndependentDirectories(t *testing.T) {
	var logs bytes.Buffer
	provider := generationProvider{list: func(_ context.Context, query provider115.FileQuery) (provider115.FilePage, error) {
		switch query.DirectoryID {
		case "42":
			if query.Cursor == "next" {
				return rootPage(videoFile("12", "第二部.mp4"), videoFile("11", "第一部.mkv")), nil
			}
			page := rootPage(videoFile("11", "第一部.mkv"),
				provider115.FileItem{ID: "51", Name: "电影", IsDirectory: true},
				provider115.FileItem{ID: "52", Name: "排除", IsDirectory: true},
				provider115.FileItem{ID: "53", Name: "无法访问", IsDirectory: true},
				provider115.FileItem{ID: "15", Name: "海报.png", FileType: "image"})
			cursor := "next"
			page.NextCursor = &cursor
			return page, nil
		case "51":
			page := rootPage(videoFile("13", "子目录.mp4"))
			page.DirectoryID = "51"
			page.Path = append(page.Path, provider115.FilePath{ID: "51", Name: "电影"})
			return page, nil
		case "53":
			return provider115.FilePage{}, provider115.ErrDirectoryNotFound
		default:
			t.Fatalf("excluded/unexpected directory queried: %q", query.DirectoryID)
			return provider115.FilePage{}, nil
		}
	}}
	service, repo, execution, result := newGenerationFixture(t, provider, &logs)
	execution.Library.IncludeSubdirectories = true
	execution.Library.ExcludeDirectoryPath = "115 网盘/媒体/排除"
	requireGeneration(t, service, execution)
	if len(repo.artifacts) != 3 || len(repo.intents) != 0 || len(result.items) != 4 {
		t.Fatalf("artifacts=%d intents=%d items=%#v", len(repo.artifacts), len(repo.intents), result.items)
	}
	if result.progress.TotalItems == nil || *result.progress.TotalItems != 4 || result.progress.ProgressPercent == nil || *result.progress.ProgressPercent != 100 {
		t.Fatalf("unexpected terminal progress %#v", result.progress)
	}
	if item := result.items[3]; item.Status != domain.ItemFailed || item.Key != "directory:53" || !strings.Contains(item.Reason, "源目录不存在") {
		t.Fatalf("unexpected directory failure %#v", item)
	}
	for _, relative := range []string{"第一部.strm", "第二部.strm", filepath.Join("电影", "子目录.strm")} {
		target := filepath.Join(execution.Library.OutputPath, relative)
		item := repo.artifacts[target]
		content := requireFile(t, target)
		if item.RelativePath != relative || item.LibraryID != execution.Library.ID || content != item.PlaybackURL+"\n" || !strings.HasPrefix(content, "https://media.example.test/p/") {
			t.Fatalf("invalid registration for %q", relative)
		}
	}
	for _, expected := range []string{
		`"msg":"STRM output directory ready"`, `"phase":"prepare_output"`,
		`"msg":"STRM source scan started"`, `"source_directory_id":"42"`, `"include_subdirectories":true`,
		`"msg":"STRM source scan finished"`, `"scan_complete":false`, `"completed_directories":2`,
		`"failed_directories":1`, `"scanned_videos":3`, `"candidate_files":3`,
		`"msg":"STRM file generation started"`, `"phase":"generate"`, `"total_items":4`,
		`"source_directory_id":"53"`, `"error_code":"provider_directory_not_found"`,
		`"outcome":"succeeded"`, `"partial_result":true`, `"generated_files":3`, `"failed_files":1`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Errorf("missing full sync log field %q", expected)
		}
	}
	if strings.Count(logs.String(), `"msg":"STRM operation finished"`) != 1 || strings.Contains(logs.String(), `"outcome":"failed"`) {
		t.Fatal("partially successful generation must have one matching terminal outcome")
	}
}

func TestGenerationLimitsFailureLogsAndRedactsUntrustedFields(t *testing.T) {
	const sourceName = "private-file-name"
	const invalidID = "private-malformed-source-id"
	const rawError = "raw repository error with cookie and https://private.example/p/token"
	var videos []provider115.FileItem
	for index := range generationSampleLimit + 2 {
		suffix := strconv.Itoa(index)
		videos = append(videos, videoFile(invalidID+suffix, sourceName+"-invalid-"+suffix+".mkv"))
		videos = append(videos, videoFile("100"+suffix, sourceName+"-valid-"+suffix+".mkv"))
	}
	videos = append(videos, videoFile("999", "can-succeed.mkv"))
	provider := generationProvider{list: func(context.Context, provider115.FileQuery) (provider115.FilePage, error) {
		return rootPage(videos...), nil
	}}
	var logOutput bytes.Buffer
	service, repo, execution, result := newGenerationFixture(t, provider, &logOutput)
	failedWrites := 0
	repo.beforeIntent = func() error {
		if failedWrites < generationSampleLimit+2 {
			failedWrites++
			return errors.New(rawError)
		}
		return nil
	}
	requireGeneration(t, service, execution)
	if len(repo.artifacts) != 1 || len(result.items) != len(videos) {
		t.Fatal("log sampling changed the number of processed items")
	}
	logs := logOutput.String()
	for _, code := range []string{"strm_invalid_source_path", "strm_registry_unavailable"} {
		if count := strings.Count(logs, `"error_code":"`+code+`"`); count != generationSampleLimit {
			t.Errorf("expected %d warnings for %s, got %d", generationSampleLimit, code, count)
		}
		if !strings.Contains(logs, `"`+code+`":5`) {
			t.Errorf("missing complete failure count for %s", code)
		}
	}
	for _, expected := range []string{`"level":"WARN"`, `"source_file_id":"1000"`, `"suppressed_failure_logs":4`, `"failed_files":10`, `"generated_files":1`} {
		if !strings.Contains(logs, expected) {
			t.Errorf("missing structured log field %q", expected)
		}
	}
	for _, private := range []string{sourceName, invalidID, rawError, "private.example", "private-account", "private-playback-code", execution.Library.OutputPath, execution.Library.DirectoryPath, execution.Library.STRMDirectDomain} {
		if strings.Contains(logs, private) {
			t.Error("sensitive path, file name, invalid source ID, or wrapped error appeared in the logs")
		}
	}
}

func TestGenerationPreservesFirstOwnerWhenExtensionsShareAnOutput(t *testing.T) {
	provider := generationProvider{list: func(context.Context, provider115.FileQuery) (provider115.FilePage, error) {
		return rootPage(videoFile("123", "样片.mkv"), videoFile("456", "样片.mp4")), nil
	}}
	service, repo, execution, result := newGenerationFixture(t, provider, io.Discard)
	execution.Library.STRMLinkFormat = "file_id"
	execution.Library.STRMExistingFilePolicy = "overwrite"
	requireGeneration(t, service, execution)

	path := filepath.Join(execution.Library.OutputPath, "样片.strm")
	if len(repo.artifacts) != 1 || repo.artifacts[path].SourceFileID != "123" || !strings.HasSuffix(requireFile(t, path), "/123\n") {
		t.Fatal("the first source must retain ownership of the shared output path")
	}
	if len(result.items) != 2 || result.items[0].Status != domain.ItemSucceeded || result.items[1].Status != domain.ItemFailed ||
		result.items[1].Key != "file:456" || result.items[1].Reason != errUnownedOutput.Error() {
		t.Fatalf("expected a conflict for the second source, got %#v", result.items)
	}
}

func TestGenerationPreservesIdentityAcrossRerunAndMissingFile(t *testing.T) {
	service, repo, execution, result := newGenerationFixture(t, singleVideoProvider(), io.Discard)
	module := service.Module()
	if !module.Eligible(execution.Library) || !module.ManualOnly {
		t.Fatal("configured proxy library should support a full sync")
	}
	unconfigured := execution.Library
	unconfigured.STRMDirectDomain = ""
	if module.Eligible(unconfigured) {
		t.Fatal("full sync should require a playback address")
	}
	direct := execution.Library
	direct.LinkType = "direct"
	if module.Eligible(direct) {
		t.Fatal("full sync should currently be available only for proxy libraries")
	}
	path := filepath.Join(execution.Library.OutputPath, "样片 1.strm")
	requireGeneration(t, service, execution)
	first := repo.artifacts[path]
	firstBytes := requireFile(t, path)
	if first.ID == "" || first.PlaybackToken == "" || first.SourceFileID != "123" || first.SourcePath != "/媒体/样片 1.mkv" {
		t.Fatal("initial registration is incomplete")
	}
	execution.Library.STRMLinkFormat = "file_id"
	execution.Library.STRMDirectDomain = "http://127.0.0.1:8080/"
	requireGeneration(t, service, execution)
	if len(repo.artifacts) != 1 || result.items[1].Status != domain.ItemSkipped || requireFile(t, path) != firstBytes {
		t.Fatal("skip policy changed an existing file")
	}
	execution.Library.STRMExistingFilePolicy = "overwrite"
	requireGeneration(t, service, execution)
	retained := repo.artifacts[path]
	updatedLink := requireFile(t, path)
	if retained.ID != first.ID || retained.PlaybackToken != first.PlaybackToken || result.items[2].Status != domain.ItemSucceeded ||
		!strings.HasPrefix(updatedLink, "http://127.0.0.1:8080/p/") || !strings.HasSuffix(updatedLink, "/123\n") {
		t.Fatal("overwrite should preserve identity and update the requested playback address and link format")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	execution.Library.STRMExistingFilePolicy = "skip"
	requireGeneration(t, service, execution)
	if repo.artifacts[path].ID != first.ID || result.items[3].Status != domain.ItemSucceeded {
		t.Fatal("missing artifact was not restored with the same identity")
	}
}

func TestGenerationPreservesExternalFilesAndConflictingOwners(t *testing.T) {
	for _, kind := range []string{"unowned", "modified", "other_library", "symlink", "created_during_generation", "modified_during_generation"} {
		t.Run(kind, func(t *testing.T) {
			var logs bytes.Buffer
			service, repo, execution, result := newGenerationFixture(t, singleVideoProvider(), &logs)
			execution.Library.STRMExistingFilePolicy = "overwrite"
			path := filepath.Join(execution.Library.OutputPath, "样片 1.strm")
			const external = "external-media"
			writeExternal := func() error { return os.WriteFile(path, []byte(external), 0o644) }
			expected := external
			switch kind {
			case "unowned":
				if err := writeExternal(); err != nil {
					t.Fatal(err)
				}
			case "modified", "other_library", "modified_during_generation":
				requireGeneration(t, service, execution)
				if kind == "other_library" {
					execution.Library.ID = "lib-other"
					expected = requireFile(t, path)
				} else if kind == "modified_during_generation" {
					execution.Library.STRMLinkFormat = "file_id"
					repo.beforeIntent = writeExternal
				} else if err := writeExternal(); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(t.TempDir(), "other.strm")
				if err := os.WriteFile(target, []byte(external), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "created_during_generation":
				repo.beforeIntent = writeExternal
			}
			if err := service.Module().Run(t.Context(), execution); !errors.Is(err, errGenerationItems) {
				t.Fatalf("expected conflicted generation, got %v", err)
			}
			if last := result.items[len(result.items)-1]; last.Status != domain.ItemFailed || requireFile(t, path) != expected {
				t.Fatalf("conflicting file changed or failure missing: %#v", last)
			}
			for _, expected := range []string{`"module":"strm"`, `"operation":"generate"`, `"request_id":"request-test"`, `"job_id":"task-test"`, `"duration_ms":`, `"outcome":"failed"`, `"conflicting_files":1`} {
				if !strings.Contains(logs.String(), expected) {
					t.Errorf("missing log attribute %q", expected)
				}
			}
			for _, secret := range []string{path, external, "private-playback-code", "media.example.test"} {
				if strings.Contains(logs.String(), secret) {
					t.Error("sensitive path, credential or content appeared in the logs")
				}
			}
		})
	}
}

func TestGenerationRecoversPublishedAndUnpublishedIntents(t *testing.T) {
	service, repo, execution, result := newGenerationFixture(t, singleVideoProvider(), io.Discard)
	path := filepath.Join(execution.Library.OutputPath, "样片 1.strm")
	repo.completeErr = errors.New("interrupted persistence")
	if err := service.Module().Run(t.Context(), execution); !errors.Is(err, errGenerationItems) {
		t.Fatalf("expected interrupted registration, got %v", err)
	}
	if len(repo.intents) != 1 || len(repo.artifacts) != 0 || requireFile(t, path) == "" {
		t.Fatal("interrupted publication was not retained for recovery")
	}
	intent := repo.intents[path]
	repo.completeErr = nil
	if verified, err := service.Module().Recover(t.Context(), execution); err != nil || !verified {
		t.Fatalf("published intent did not recover: verified=%t err=%v", verified, err)
	}
	if len(repo.intents) != 0 || repo.artifacts[path].ID != intent.Artifact.ID {
		t.Fatal("recovered publication did not preserve identity")
	}
	previous := requireFile(t, path)
	intent.PreviousContent = &previous
	intent.Artifact.PlaybackURL = playbackURL("https://new.example.test", intent.Artifact, "file_id")
	repo.intents[path] = intent
	if verified, err := service.Module().Recover(t.Context(), execution); err != nil || !verified || len(repo.intents) != 0 {
		t.Fatalf("unpublished intent was not released: verified=%t err=%v", verified, err)
	}
	repo.intents[path] = intent
	if err := os.WriteFile(path, []byte("administrator content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if verified, err := service.Module().Recover(t.Context(), execution); err != nil || verified || len(repo.intents) != 1 {
		t.Fatalf("ambiguous intent was not preserved: verified=%t err=%v", verified, err)
	}
	if len(result.logs) == 0 || !strings.Contains(result.logs[0], "失败明细") || requireFile(t, path) != "administrator content" {
		t.Fatal("recovery modified external content or lost the failure explanation")
	}
}

func TestGenerationRecoversLegacyIntentsWithoutAdoptingExternalContent(t *testing.T) {
	service, repo, execution, _ := newGenerationFixture(t, singleVideoProvider(), io.Discard)
	path := filepath.Join(execution.Library.OutputPath, "样片 1.strm")
	repo.completeErr = errors.New("interrupted persistence")
	if err := service.Module().Run(t.Context(), execution); !errors.Is(err, errGenerationItems) {
		t.Fatalf("expected interrupted registration, got %v", err)
	}
	repo.completeErr = nil
	legacy := repo.intents[path]
	legacy.Artifact.PlaybackURL = ""
	legacy.Legacy = true
	repo.intents[path] = legacy
	generated := requireFile(t, path)
	modified := strings.TrimSuffix(generated, "\n") + "?added=1\n"
	if err := os.WriteFile(path, []byte(modified), 0o644); err != nil {
		t.Fatal(err)
	}
	if verified, err := service.Module().Recover(t.Context(), execution); err != nil || verified || len(repo.intents) != 1 || len(repo.artifacts) != 0 {
		t.Fatalf("legacy intent accepted altered content: verified=%t err=%v", verified, err)
	}
	if err := os.WriteFile(path, []byte(generated), 0o644); err != nil {
		t.Fatal(err)
	}
	if verified, err := service.Module().Recover(t.Context(), execution); err != nil || !verified || len(repo.intents) != 0 {
		t.Fatalf("published legacy intent did not recover: verified=%t err=%v", verified, err)
	}
	registered := repo.artifacts[path]
	if registered.PlaybackURL+"\n" != generated || registered.ID != legacy.Artifact.ID {
		t.Fatal("legacy recovery did not register the existing link and identity")
	}

	registered.PlaybackURL = ""
	repo.artifacts[path] = registered
	repo.intents[path] = legacy
	if state, content := artifactContent(registered); state != "present" || content == nil || *content != generated {
		t.Fatalf("legacy registered link could not be previewed: state=%s", state)
	}
	execution.Library.STRMDirectDomain = "https://new.example.test"
	execution.Library.STRMExistingFilePolicy = "overwrite"
	requireGeneration(t, service, execution)
	updated := requireFile(t, path)
	if len(repo.intents) != 0 || repo.artifacts[path].ID != registered.ID || repo.artifacts[path].PlaybackURL+"\n" != updated ||
		!strings.HasPrefix(updated, "https://new.example.test/p/") {
		t.Fatal("legacy overwrite did not retain ownership or apply the configured address")
	}
}

func TestGenerationRejectsChangedAccountBeforePublishingAnyFile(t *testing.T) {
	provider := generationProvider{list: func(_ context.Context, query provider115.FileQuery) (provider115.FilePage, error) {
		page := rootPage(videoFile("123", "样片 1.mkv"))
		if query.Cursor == "" {
			cursor := "next"
			page.NextCursor = &cursor
		} else {
			page.AccountID = "different-account"
		}
		return page, nil
	}}
	service, repo, execution, result := newGenerationFixture(t, provider, io.Discard)
	if err := service.Module().Run(t.Context(), execution); !errors.Is(err, errAccountChanged) {
		t.Fatalf("expected account-change failure, got %v", err)
	}
	_, err := os.Stat(filepath.Join(execution.Library.OutputPath, "样片 1.strm"))
	if !errors.Is(err, fs.ErrNotExist) || len(repo.artifacts) != 0 || len(result.logs) != 1 {
		t.Fatal("changed account resulted in partial output or omitted task error")
	}
}

func TestGenerationSerializesLibraryOutputsThroughRootAlias(t *testing.T) {
	service, repo, first, firstResult := newGenerationFixture(t, singleVideoProvider(), io.Discard)
	_, _, second, secondResult := newGenerationFixture(t, singleVideoProvider(), io.Discard)
	second.Library.ID = "lib-second"
	second.Library.OutputPath = filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(first.Library.OutputPath, second.Library.OutputPath); err != nil {
		t.Fatal(err)
	}
	var running sync.WaitGroup
	for _, execution := range []jobs.Execution{first, second} {
		running.Go(func() {
			err := service.Module().Run(t.Context(), execution)
			if err != nil && !errors.Is(err, errGenerationItems) {
				t.Errorf("unexpected competing generation result: %v", err)
			}
		})
	}
	running.Wait()
	if len(repo.artifacts) != 1 || len(repo.intents) != 0 || len(firstResult.items) != 1 || len(secondResult.items) != 1 {
		t.Fatal("overlapping targets did not converge on one artifact")
	}
	if (firstResult.items[0].Status == domain.ItemSucceeded) == (secondResult.items[0].Status == domain.ItemSucceeded) {
		t.Fatal("exactly one library should own the artifact")
	}
}

func TestGenerationRejectsOutputDirectoryLinks(t *testing.T) {
	provider := generationProvider{list: func(_ context.Context, query provider115.FileQuery) (provider115.FilePage, error) {
		if query.DirectoryID == "42" {
			return rootPage(provider115.FileItem{ID: "43", Name: "链接目录", IsDirectory: true}), nil
		}
		page := rootPage(videoFile("123", "样片 1.mkv"))
		page.DirectoryID = "43"
		page.Path = append(page.Path, provider115.FilePath{ID: "43", Name: "链接目录"})
		return page, nil
	}}
	service, repo, execution, result := newGenerationFixture(t, provider, io.Discard)
	execution.Library.IncludeSubdirectories = true
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(execution.Library.OutputPath, "链接目录")); err != nil {
		t.Fatal(err)
	}
	if err := service.Module().Run(t.Context(), execution); !errors.Is(err, errGenerationItems) {
		t.Fatalf("linked subdirectory was not rejected: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || len(repo.artifacts) != 0 || len(result.items) != 1 || result.items[0].Status != domain.ItemFailed {
		t.Fatal("linked output was touched or failure was not reported")
	}
}

func TestGenerationCancellationLeavesIntentForSafeRetry(t *testing.T) {
	var logs bytes.Buffer
	service, repo, execution, result := newGenerationFixture(t, singleVideoProvider(), &logs)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	repo.beforeIntent = func() error { cancel(); return nil }
	if err := service.Module().Run(ctx, execution); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled generation, got %v", err)
	}
	_, err := os.Stat(filepath.Join(execution.Library.OutputPath, "样片 1.strm"))
	if !errors.Is(err, fs.ErrNotExist) || len(repo.intents) != 1 || len(repo.artifacts) != 0 || len(result.items) != 0 || !strings.Contains(logs.String(), `"outcome":"canceled"`) {
		t.Fatal("generation published after cancel or omitted recovery evidence")
	}
	repo.beforeIntent = nil
	requireGeneration(t, service, execution)
	if len(repo.intents) != 0 || len(repo.artifacts) != 1 || len(result.items) != 1 || result.items[0].Status != domain.ItemSucceeded {
		t.Fatal("canceled generation was not safely retried")
	}
}

func TestGenerationRejectsOutputRootReplacedDuringWrite(t *testing.T) {
	service, repo, execution, result := newGenerationFixture(t, singleVideoProvider(), io.Discard)
	root := execution.Library.OutputPath
	moved := filepath.Join(t.TempDir(), "moved")
	repo.beforeIntent = func() error {
		if err := os.Rename(root, moved); err != nil {
			return err
		}
		return os.Mkdir(root, 0o755)
	}
	if err := service.Module().Run(t.Context(), execution); !errors.Is(err, errGenerationItems) {
		t.Fatalf("expected write to fail after output directory replacement, got %v", err)
	}
	for _, path := range []string{root, moved} {
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 0 {
			t.Fatalf("generation touched output after the directory was replaced: %v", err)
		}
	}
	if len(repo.artifacts) != 0 || len(repo.intents) != 1 || len(result.items) != 1 || result.items[0].Status != domain.ItemFailed {
		t.Fatal("replaced output directory was recorded as successfully generated")
	}
}
