package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStructuredLogAndRotation(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "app.log")
	// Seed expired backups to verify the configured retention policy on rotation.
	oldName := filepath.Join(directory, "app-"+time.Now().Add(-8*24*time.Hour).UTC().Format("2006-01-02T15-04-05.000")+".log")
	if err := os.WriteFile(oldName, []byte("expired"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	logger, closer, err := NewLogger(Config{LogFile: filename}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("started", "module", "app", "operation", "startup", "outcome", "succeeded")
	var entry map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["module"] != "app" || entry["operation"] != "startup" || entry["outcome"] != "succeeded" {
		t.Fatalf("missing log fields: %v", entry)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	// A large existing active file triggers real lumberjack rotation on the next write.
	if err := os.WriteFile(filename, []byte(strings.Repeat("x", 10*1024*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	logger, closer, err = NewLogger(Config{LogFile: filename}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("rotated")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := os.Stat(oldName)
		if os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired log was not removed")
		}
		runtime.Gosched()
	}
	entries, err := filepath.Glob(filepath.Join(directory, "app-*.log"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("rotation backups: %v, %v", entries, err)
	}
	active, err := os.ReadFile(filename)
	if err != nil || !strings.Contains(string(active), "rotated") {
		t.Fatalf("new active log: %s, %v", active, err)
	}
}
