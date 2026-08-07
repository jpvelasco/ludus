package buildlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testTime() time.Time {
	return time.Date(2026, 6, 19, 15, 4, 5, 0, time.UTC)
}

func TestNew_CreatesTimestampedFile(t *testing.T) {
	dir := t.TempDir()

	lg, err := New(dir, "run", testTime())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer lg.Close()

	want := filepath.Join(dir, "ludus-2026-06-19T15-04-05-run.log")
	if lg.Path() != want {
		t.Errorf("Path() = %q, want %q", lg.Path(), want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected log file to exist: %v", err)
	}
}

func TestNew_CreatesDirIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")

	lg, err := New(dir, "run", testTime())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer lg.Close()

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected log dir to be created: %v", err)
	}
}

func TestWriter_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	lg, err := New(dir, "run", testTime())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := lg.Writer().Write([]byte("hello build\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(lg.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello build") {
		t.Errorf("log file missing written content, got: %q", data)
	}
}

func TestNew_DoesNotClobberSameSecond(t *testing.T) {
	dir := t.TempDir()
	ts := testTime()

	lg1, err := New(dir, "build", ts)
	if err != nil {
		t.Fatalf("New() #1 error = %v", err)
	}
	_, _ = lg1.Writer().Write([]byte("first\n"))
	if err := lg1.Close(); err != nil {
		t.Fatal(err)
	}

	// Same runName + same second must NOT overwrite the first log.
	lg2, err := New(dir, "build", ts)
	if err != nil {
		t.Fatalf("New() #2 error = %v", err)
	}
	defer lg2.Close()
	if lg2.Path() == lg1.Path() {
		t.Errorf("second log reused path %q; earlier log would be truncated", lg1.Path())
	}

	data, err := os.ReadFile(lg1.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "first") {
		t.Errorf("first log was clobbered, got: %q", data)
	}
}

func TestSection_NilReceiverIsNoop(t *testing.T) {
	var lg *Logger
	lg.Section("never written") // must not panic
	if err := lg.Close(); err != nil {
		t.Errorf("Close() on nil logger = %v, want nil", err)
	}

	empty := &Logger{}
	empty.Section("never written")
	if err := empty.Close(); err != nil {
		t.Errorf("Close() on empty logger = %v, want nil", err)
	}
}

func TestNew_MkdirAllFails(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(filepath.Join(blocker, "logs"), "run", testTime())
	if err == nil {
		t.Fatal("expected error when log dir path is a file")
	}
	if !strings.Contains(err.Error(), "creating log dir") {
		t.Errorf("error = %v, want it to describe the dir failure", err)
	}
}

func TestSection_WritesHeader(t *testing.T) {
	dir := t.TempDir()
	lg, err := New(dir, "run", testTime())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	lg.Section("Build Unreal Engine")
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(lg.Path())
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "Build Unreal Engine") {
		t.Errorf("section header missing stage name, got: %q", got)
	}
	if !strings.Contains(got, "=====") {
		t.Errorf("section header missing delimiter, got: %q", got)
	}
}
