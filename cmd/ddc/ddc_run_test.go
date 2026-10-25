package ddc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	internalddc "github.com/jpvelasco/ludus/internal/ddc"
)

func TestRunStatusErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		opts []globals.GlobalOption
		want string
	}{
		{
			name: "invalid ddc mode",
			cfg:  &config.Config{},
			opts: []globals.GlobalOption{globals.WithDDCMode("bogus")},
			want: "invalid DDC mode",
		},
		{
			name: "relative zen path",
			cfg:  &config.Config{DDC: config.DDCConfig{Mode: internalddc.ModeZen, ZenPath: "relative/zen"}},
			want: "must be absolute",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, tt.cfg, tt.opts...)
			err := runStatus(statusCmd, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runStatus() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestRunStatusNoneMode(t *testing.T) {
	globals.SetGlobals(t, &config.Config{DDC: config.DDCConfig{Mode: internalddc.ModeNone}})

	output, err := captureDDCStdout(t, func() error { return runStatus(statusCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Mode: none", "Size: 0 B"} {
		if !strings.Contains(output, want) {
			t.Errorf("output %q does not contain %q", output, want)
		}
	}
}

func TestRunCleanAndPruneWithData(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		json    bool
		run     func() error
		want    []string
	}{
		{
			name:    "clean human",
			fixture: "clean",
			run:     func() error { return runClean(cleanCmd, nil) },
			want:    []string{"DDC cleaned: 5 B freed"},
		},
		{
			name:    "clean json",
			fixture: "clean",
			json:    true,
			run:     func() error { return runClean(cleanCmd, nil) },
			want:    []string{`"success":true`, `"bytes_freed":5`},
		},
		{
			name:    "prune human",
			fixture: "prune",
			run:     func() error { return runPrune(pruneCmd, nil) },
			want:    []string{"DDC pruned: 1.0 KB freed (entries older than 7 days)"},
		},
		{
			name:    "prune json",
			fixture: "prune",
			json:    true,
			run:     func() error { return runPrune(pruneCmd, nil) },
			want:    []string{`"success":true`, `"bytes_freed":1024`, `"max_age_days":7`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.fixture == "clean" {
				writeCacheFile(t, filepath.Join(dir, "cache.bin"), 5)
			} else {
				writeOldFile(t, dir, 1024)
			}

			pruneDays = 7
			t.Cleanup(func() { pruneDays = 30 })
			globals.SetGlobals(t, localDDCConfig(dir),
				globals.WithJSONOutput(tt.json), globals.WithNoLogs(true))

			output, err := captureDDCStdout(t, tt.run)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("output %q does not contain %q", output, want)
				}
			}
		})
	}
}

func TestRunCleanAndPruneErrors(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Config
		pruneDays int
		run       func() error
		want      string
	}{
		{
			name: "clean relative local path",
			cfg:  &config.Config{DDC: config.DDCConfig{Mode: internalddc.ModeLocal, LocalPath: "relative/ddc"}},
			run:  func() error { return runClean(cleanCmd, nil) },
			want: "must be absolute",
		},
		{
			name: "prune relative local path",
			cfg:  &config.Config{DDC: config.DDCConfig{Mode: internalddc.ModeLocal, LocalPath: "relative/ddc"}},
			run:  func() error { return runPrune(pruneCmd, nil) },
			want: "must be absolute",
		},
		{
			name:      "prune invalid days",
			cfg:       localDDCConfig(t.TempDir()),
			pruneDays: 0,
			run:       func() error { return runPrune(pruneCmd, nil) },
			want:      "pruning DDC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pruneDays = tt.pruneDays
			t.Cleanup(func() { pruneDays = 30 })
			globals.SetGlobals(t, tt.cfg, globals.WithNoLogs(true))

			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("command error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// localDDCConfig returns a config that explicitly selects the legacy local
// backend at dir. Tests must pin the mode: with the default zen mode these
// commands operate on the Zen store instead (#699).
func localDDCConfig(dir string) *config.Config {
	return &config.Config{DDC: config.DDCConfig{Mode: internalddc.ModeLocal, LocalPath: dir}}
}

// TestRunCleanZenModeTargetsZenStore reproduces #699: in zen mode (the
// default) clean must reset the Zen store directory, not the legacy cache.
func TestRunCleanZenModeTargetsZenStore(t *testing.T) {
	zenDir, localDir := t.TempDir(), t.TempDir()
	writeCacheFile(t, filepath.Join(zenDir, "zen.dat"), 7)
	writeCacheFile(t, filepath.Join(localDir, "legacy.udd"), 9)

	globals.SetGlobals(t, &config.Config{DDC: config.DDCConfig{
		Mode: internalddc.ModeZen, ZenPath: zenDir, LocalPath: localDir,
	}}, globals.WithNoLogs(true))

	output, err := captureDDCStdout(t, func() error { return runClean(cleanCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "DDC cleaned: 7 B freed") {
		t.Errorf("output = %q, want the Zen store cleaned", output)
	}
	assertDDCDirEntries(t, zenDir, 0)
	assertDDCDirEntries(t, localDir, 1) // legacy cache untouched
}

// TestRunStatusZenModeReportsZenPath pins the status side of #699: status
// reports the active backend path even when a legacy path is also set.
func TestRunStatusZenModeReportsZenPath(t *testing.T) {
	zenDir := t.TempDir()
	writeCacheFile(t, filepath.Join(zenDir, "zen.dat"), 3)

	globals.SetGlobals(t, &config.Config{DDC: config.DDCConfig{
		Mode: internalddc.ModeZen, ZenPath: zenDir, LocalPath: t.TempDir(),
	}}, globals.WithNoLogs(true))

	output, err := captureDDCStdout(t, func() error { return runStatus(statusCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Mode: zen", "Path: " + zenDir, "Size: 3 B"} {
		if !strings.Contains(output, want) {
			t.Errorf("output %q does not contain %q", output, want)
		}
	}
}

// TestRunPruneZenModeUnsupported pins the prune semantics for zen mode
// (#699): the Zen store is not a per-entry file tree, so age-based pruning
// must refuse instead of deleting files out of a live store.
func TestRunPruneZenModeUnsupported(t *testing.T) {
	zenDir := t.TempDir()
	writeCacheFile(t, filepath.Join(zenDir, "zen.dat"), 5)

	globals.SetGlobals(t, &config.Config{DDC: config.DDCConfig{
		Mode: internalddc.ModeZen, ZenPath: zenDir, LocalPath: t.TempDir(),
	}}, globals.WithNoLogs(true))

	err := runPrune(pruneCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("runPrune() error = %v, want the zen unsupported error", err)
	}
	if !strings.Contains(err.Error(), "ludus ddc clean") {
		t.Errorf("error = %v, want it to point at clean for a store reset", err)
	}
	assertDDCDirEntries(t, zenDir, 1) // refusing must leave the store intact
}

// TestRunCleanAndPruneDryRun reproduces #700: --dry-run must preview the
// destructive operations without removing anything.
func TestRunCleanAndPruneDryRun(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		json    bool
		run     func() error
		want    []string
	}{
		{
			name:    "clean previews",
			fixture: "clean",
			run:     func() error { return runClean(cleanCmd, nil) },
			want:    []string{"DRY RUN: DDC clean", "Would free: 5 B"},
		},
		{
			name:    "clean json preview",
			fixture: "clean",
			json:    true,
			run:     func() error { return runClean(cleanCmd, nil) },
			want:    []string{`"dry_run":true`, `"bytes_freed":5`},
		},
		{
			name:    "prune previews",
			fixture: "prune",
			run:     func() error { return runPrune(pruneCmd, nil) },
			want:    []string{"DRY RUN: DDC prune", "Would free : 1.0 KB"},
		},
		{
			name:    "prune json preview",
			fixture: "prune",
			json:    true,
			run:     func() error { return runPrune(pruneCmd, nil) },
			want:    []string{`"dry_run":true`, `"bytes_freed":1024`, `"max_age_days":7`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.fixture == "clean" {
				writeCacheFile(t, filepath.Join(dir, "cache.bin"), 5)
			} else {
				writeOldFile(t, dir, 1024)
			}

			pruneDays = 7
			t.Cleanup(func() { pruneDays = 30 })
			globals.SetGlobals(t, localDDCConfig(dir),
				globals.WithDryRun(true), globals.WithJSONOutput(tt.json), globals.WithNoLogs(true))

			output, err := captureDDCStdout(t, tt.run)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("output %q does not contain %q", output, want)
				}
			}
			assertDDCDirEntries(t, dir, 1) // dry-run must not remove cache content
		})
	}
}

// TestRunCleanAndPruneDisabledNoop pins the mode=none behavior: with DDC
// disabled there is no cache to operate on, so neither command may touch the
// configured paths.
func TestRunCleanAndPruneDisabledNoop(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{"clean", func() error { return runClean(cleanCmd, nil) }, "nothing to clean"},
		{"prune", func() error { return runPrune(pruneCmd, nil) }, "nothing to prune"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localDir := t.TempDir()
			writeCacheFile(t, filepath.Join(localDir, "legacy.udd"), 4)

			globals.SetGlobals(t, &config.Config{DDC: config.DDCConfig{
				Mode: internalddc.ModeNone, LocalPath: localDir,
			}}, globals.WithNoLogs(true))

			output, err := captureDDCStdout(t, tt.run)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, tt.want) {
				t.Errorf("output = %q, want it to contain %q", output, tt.want)
			}
			assertDDCDirEntries(t, localDir, 1) // configured paths untouched
		})
	}
}

func assertDDCDirEntries(t *testing.T, dir string, want int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if len(entries) != want {
		t.Errorf("%s has %d entries, want %d", dir, len(entries), want)
	}
}

func writeCacheFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeOldFile(t *testing.T, dir string, size int) {
	t.Helper()
	path := filepath.Join(dir, "old.bin")
	writeCacheFile(t, path, size)
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
}
