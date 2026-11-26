// intent_test.go：验证下载意图的路径、字节边界和相邻阶段约束，避免存入不可安全恢复的身份。

package sidecar

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// testIntent 提供合法的最小意图，不依赖磁盘或数据库。
func testIntent(t *testing.T) Intent {
	t.Helper()
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	root := t.TempDir()
	return Intent{ID: "intent", JobID: "job", Attempt: 1, LibraryID: "library", AccountID: "account",
		SourceFileID: "10", SourceDirectoryID: "20", Kind: KindNFO, OutputRoot: root, RelativePath: "movie.nfo",
		TargetPath: filepath.Join(root, "movie.nfo"), State: StateReserved, Revision: 1, CreatedAt: now, UpdatedAt: now}
}

// TestIntentRejectsUnrecoverableMetadata 覆盖路径穿越、缺失来源、临时文件异常身份和不同附件上限。
func TestIntentRejectsUnrecoverableMetadata(t *testing.T) {
	cases := map[string]func(*Intent){
		"missing directory": func(i *Intent) { i.SourceDirectoryID = "" },
		"missing run":       func(i *Intent) { i.Attempt = 0 },
		"invalid kind":      func(i *Intent) { i.Kind = "subtitle" },
		"relative root":     func(i *Intent) { i.OutputRoot = "relative" },
		"traversal": func(i *Intent) {
			i.RelativePath = "../movie.nfo"
			i.TargetPath = filepath.Join(i.OutputRoot, i.RelativePath)
		},
		"unclean":      func(i *Intent) { i.RelativePath = "dir/../movie.nfo" },
		"wrong target": func(i *Intent) { i.TargetPath += ".other" },
		"backslash": func(i *Intent) {
			i.RelativePath = `dir\movie.nfo`
			i.TargetPath = filepath.Join(i.OutputRoot, i.RelativePath)
		},
		"nul":                      func(i *Intent) { i.RelativePath += "\x00" },
		"temporary traversal":      func(i *Intent) { i.TemporaryName = "../temporary" },
		"temporary equals final":   func(i *Intent) { i.TemporaryName = "movie.nfo" },
		"reserved inode":           func(i *Intent) { i.TemporaryDevice = "1"; i.TemporaryInode = "2" },
		"negative expected":        func(i *Intent) { n := int64(-1); i.ExpectedBytes = &n },
		"nfo limit":                func(i *Intent) { n := MaxNFOBytes + 1; i.ExpectedBytes = &n },
		"image limit":              func(i *Intent) { i.Kind = KindImage; n := MaxImageBytes + 1; i.ExpectedBytes = &n },
		"negative completed":       func(i *Intent) { i.CompletedBytes = -1 },
		"reserved completed":       func(i *Intent) { i.CompletedBytes = 1 },
		"time before created":      func(i *Intent) { i.UpdatedAt = i.CreatedAt.Add(-time.Nanosecond) },
		"missing time":             func(i *Intent) { i.CreatedAt = time.Time{} },
		"missing revision":         func(i *Intent) { i.Revision = 0 },
		"unknown state":            func(i *Intent) { i.State = "downloading" },
		"staging missing identity": func(i *Intent) { i.State = StateStaging; i.TemporaryName = ".sidecar-temp" },
		"staged wrong length": func(i *Intent) {
			i.State = StateStaged
			i.TemporaryName = ".sidecar-temp"
			i.TemporaryDevice = "0"
			i.TemporaryInode = "2"
			n := int64(1)
			i.ExpectedBytes = &n
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			i := testIntent(t)
			mutate(&i)
			if err := i.Validate(); !errors.Is(err, ErrConflict) {
				t.Fatalf("unsafe metadata accepted: %v", err)
			}
		})
	}
	for _, kind := range []Kind{KindNFO, KindImage} {
		i := testIntent(t)
		i.Kind = kind
		n := MaxNFOBytes
		if kind == KindImage {
			n = MaxImageBytes
		}
		i.ExpectedBytes = &n
		if err := i.Validate(); err != nil {
			t.Fatalf("valid maximum %s: %v", kind, err)
		}
	}
}

// TestIntentStageFactsAreImmutable 验证不能跳过阶段、替换已记录临时身份、倒退版本或改写已暂存字节数。
func TestIntentStageFactsAreImmutable(t *testing.T) {
	reserved := testIntent(t)
	reserved.TemporaryName = ".sidecar-reserved"
	staging := reserved
	staging.State, staging.Revision = StateStaging, 2
	staging.TemporaryDevice, staging.TemporaryInode = "0", "2"
	if err := reserved.CanAdvance(staging); err != nil {
		t.Fatal(err)
	}
	staged := staging
	staged.State, staged.Revision, staged.CompletedBytes = StateStaged, 3, 12
	if err := staging.CanAdvance(staged); err != nil {
		t.Fatal(err)
	}
	published := staged
	published.State, published.Revision = StatePublished, 4
	if err := staged.CanAdvance(published); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name          string
		before, after Intent
	}{
		{"skipped stage", reserved, staged},
		{"regression", staged, staging},
		{"repeat stage", staging, staging},
		{"repeat published", published, published},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.before.CanAdvance(tc.after); !errors.Is(err, ErrConflict) {
				t.Fatalf("invalid phase accepted: %v", err)
			}
		})
	}
	for _, mutate := range []func(*Intent){
		func(i *Intent) { i.TemporaryName = ".sidecar-other" },
		func(i *Intent) { i.TemporaryDevice = "3" },
		func(i *Intent) { i.TemporaryInode = "4" },
		func(i *Intent) { i.CompletedBytes++ },
		func(i *Intent) { i.UpdatedAt = i.CreatedAt.Add(-time.Second) },
	} {
		changed := published
		mutate(&changed)
		if err := staged.CanAdvance(changed); !errors.Is(err, ErrConflict) {
			t.Fatalf("staged fact mutation accepted: %v", err)
		}
	}
	otherName := staging
	otherName.TemporaryName = ".sidecar-other"
	if err := reserved.CanAdvance(otherName); !errors.Is(err, ErrConflict) {
		t.Fatalf("reserved temporary name changed: %v", err)
	}
}
