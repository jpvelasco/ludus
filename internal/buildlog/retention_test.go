package buildlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeLogFiles creates n dummy .log files with increasing modtimes and returns
// their names oldest-first.
func writeLogFiles(t *testing.T, dir string, n int) []string {
	t.Helper()
	var names []string
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		name := filepath.Join(dir, "log"+string(rune('a'+i))+".log")
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(name, mt, mt); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return names
}

func countLogs(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			n++
		}
	}
	return n
}

func TestPrune_KeepsNewestN(t *testing.T) {
	dir := t.TempDir()
	names := writeLogFiles(t, dir, 5)

	if err := Prune(dir, 2); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if got := countLogs(t, dir); got != 2 {
		t.Errorf("after prune got %d logs, want 2", got)
	}
	// Oldest three should be gone; newest two should remain.
	for _, old := range names[:3] {
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Errorf("expected %s pruned, still present", old)
		}
	}
	for _, keep := range names[3:] {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("expected %s kept, missing: %v", keep, err)
		}
	}
}

func TestPrune_NoopWhenUnderLimit(t *testing.T) {
	dir := t.TempDir()
	writeLogFiles(t, dir, 3)

	if err := Prune(dir, 10); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if got := countLogs(t, dir); got != 3 {
		t.Errorf("got %d logs, want 3 (no prune under limit)", got)
	}
}

func TestPrune_MissingDirIsNoError(t *testing.T) {
	if err := Prune(filepath.Join(t.TempDir(), "does-not-exist"), 5); err != nil {
		t.Errorf("Prune() on missing dir should be nil, got: %v", err)
	}
}
