package ddc

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	internalddc "github.com/jpvelasco/ludus/internal/ddc"
)

func TestStatusPath(t *testing.T) {
	resetDDCGlobals(t)
	localPath := filepath.Join(t.TempDir(), "local")
	zenPath := filepath.Join(t.TempDir(), "zen")
	globals.Cfg = &config.Config{}
	globals.Cfg.DDC.LocalPath = localPath
	globals.Cfg.DDC.ZenPath = zenPath

	tests := []struct {
		mode string
		want string
	}{
		{internalddc.ModeZen, zenPath},
		{internalddc.ModeLocal, localPath},
		{internalddc.ModeNone, ""},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got, err := statusPath(tt.mode)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("statusPath(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestRunStatusHumanAndJSON(t *testing.T) {
	tests := []struct {
		name string
		json bool
		want []string
	}{
		{"human", false, []string{"DDC Status", "Mode: local", "Size: 5 B"}},
		{"json", true, []string{`"mode":"local"`, `"size_bytes":5`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDDCGlobals(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "cache.bin"), []byte("12345"), 0o644); err != nil {
				t.Fatal(err)
			}
			globals.Cfg = &config.Config{}
			globals.Cfg.DDC.Mode = internalddc.ModeLocal
			globals.Cfg.DDC.LocalPath = dir
			globals.JSONOutput = tt.json

			output, err := captureDDCStdout(t, func() error { return runStatus(statusCmd, nil) })
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

func TestRunCleanAndPruneEmptyCache(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{"clean", func() error { return runClean(cleanCmd, nil) }, "already empty"},
		{"prune", func() error { return runPrune(pruneCmd, nil) }, "No DDC entries older than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDDCGlobals(t)
			globals.Cfg = &config.Config{}
			globals.Cfg.DDC.LocalPath = t.TempDir()
			output, err := captureDDCStdout(t, tt.run)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, tt.want) {
				t.Fatalf("output = %q, want it to contain %q", output, tt.want)
			}
		})
	}
}

func TestRunWarmupRejectsDisabledDDC(t *testing.T) {
	resetDDCGlobals(t)
	globals.Cfg = &config.Config{}
	globals.Cfg.DDC.Mode = internalddc.ModeNone
	err := runWarmup(warmupCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "requires DDC to be enabled") {
		t.Fatalf("runWarmup() error = %v, want disabled DDC error", err)
	}
}

func resetDDCGlobals(t *testing.T) {
	t.Helper()
	previousCfg := globals.Cfg
	previousMode := globals.DDCMode
	previousJSON := globals.JSONOutput
	previousDryRun := globals.DryRun
	previousPruneDays := pruneDays
	globals.DDCMode = ""
	globals.JSONOutput = false
	globals.DryRun = false
	pruneDays = 30
	t.Cleanup(func() {
		globals.Cfg = previousCfg
		globals.DDCMode = previousMode
		globals.JSONOutput = previousJSON
		globals.DryRun = previousDryRun
		pruneDays = previousPruneDays
	})
}

func captureDDCStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	t.Cleanup(func() { os.Stdout = previous })
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}
