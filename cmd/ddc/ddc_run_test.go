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
			globals.SetGlobals(t, &config.Config{DDC: config.DDCConfig{LocalPath: dir}},
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
			cfg:  &config.Config{DDC: config.DDCConfig{LocalPath: "relative/ddc"}},
			run:  func() error { return runClean(cleanCmd, nil) },
			want: "must be absolute",
		},
		{
			name: "prune relative local path",
			cfg:  &config.Config{DDC: config.DDCConfig{LocalPath: "relative/ddc"}},
			run:  func() error { return runPrune(pruneCmd, nil) },
			want: "must be absolute",
		},
		{
			name:      "prune invalid days",
			cfg:       &config.Config{DDC: config.DDCConfig{LocalPath: t.TempDir()}},
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
