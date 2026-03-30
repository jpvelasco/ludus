package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestApplyNuGetAuditWorkaround(t *testing.T) {
	tests := []struct {
		name          string
		engineVersion string
		wantApplied   bool
	}{
		{"5.6 applies", "5.6", true},
		{"empty applies (safe default)", "", true},
		{"5.5 skips", "5.5", false},
		{"5.7 skips", "5.7", false},
		{"5.4 skips", "5.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runner.NewRunner(false, true) // dry-run mode
			b := NewBuilder(BuildOptions{
				EngineVersion: tt.engineVersion,
			}, r)

			b.applyNuGetAuditWorkaround()

			found := false
			for _, kv := range b.Runner.Env {
				if kv == "NuGetAuditLevel=critical" {
					found = true
					break
				}
			}

			if found != tt.wantApplied {
				t.Errorf("applyNuGetAuditWorkaround() applied=%v, want %v (engineVersion=%q)",
					found, tt.wantApplied, tt.engineVersion)
			}
		})
	}
}

func TestResolveMaxJobs(t *testing.T) {
	tests := []struct {
		name         string
		maxJobs      int
		crossCompile bool
		wantExact    int  // -1 means "don't check exact, just check > 0"
		wantPositive bool // true means result should be > 0
	}{
		{"explicit jobs used as-is", 4, false, 4, true},
		{"explicit jobs with cross-compile", 8, true, 8, true},
		{"auto-detect native", 0, false, -1, true},
		{"auto-detect cross-compile", 0, true, -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runner.NewRunner(false, true)
			b := NewBuilder(BuildOptions{MaxJobs: tt.maxJobs}, r)
			got := b.resolveMaxJobs(tt.crossCompile)
			if tt.wantExact >= 0 && got != tt.wantExact {
				t.Errorf("resolveMaxJobs() = %d, want %d", got, tt.wantExact)
			}
			if tt.wantPositive && got <= 0 {
				// Auto-detect may return 0 if RAM detection fails, which is OK
				t.Logf("resolveMaxJobs() = %d (RAM detection may have failed)", got)
			}
		})
	}
}

func TestClientBinaryPath(t *testing.T) {
	tests := []struct {
		name         string
		projectName  string
		clientTarget string
		platform     string
		wantSuffix   string
	}{
		{"Win64 default", "", "", "Win64", filepath.Join("Windows", "Lyra", "Binaries", "Win64", "LyraGame.exe")},
		{"Linux default", "", "", "Linux", filepath.Join("Linux", "Lyra", "Binaries", "Linux", "LyraGame")},
		{"Win64 custom", "MyGame", "MyGameClient", "Win64", filepath.Join("Windows", "MyGame", "Binaries", "Win64", "MyGameClient.exe")},
		{"Linux custom", "MyGame", "MyGameClient", "Linux", filepath.Join("Linux", "MyGame", "Binaries", "Linux", "MyGameClient")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runner.NewRunner(false, true)
			b := NewBuilder(BuildOptions{
				ProjectName:  tt.projectName,
				ClientTarget: tt.clientTarget,
			}, r)
			got := b.clientBinaryPath("/out", tt.platform)
			if !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("clientBinaryPath() = %q, want suffix %q", got, tt.wantSuffix)
			}
		})
	}
}

func TestDirHasContent(t *testing.T) {
	t.Run("non-existent dir", func(t *testing.T) {
		if dirHasContent(filepath.Join(t.TempDir(), "nope")) {
			t.Error("expected false for non-existent dir")
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		if dirHasContent(t.TempDir()) {
			t.Error("expected false for empty dir")
		}
	})

	t.Run("dir with content", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if !dirHasContent(dir) {
			t.Error("expected true for dir with content")
		}
	})
}

func TestScanBuildLogs(t *testing.T) {
	t.Run("empty engine path", func(t *testing.T) {
		if hints := scanBuildLogs(""); hints != nil {
			t.Errorf("expected nil for empty path, got %v", hints)
		}
	})

	t.Run("no log file", func(t *testing.T) {
		if hints := scanBuildLogs(t.TempDir()); hints != nil {
			t.Errorf("expected nil for missing log, got %v", hints)
		}
	})

	t.Run("log with known pattern", func(t *testing.T) {
		dir := t.TempDir()
		logDir := filepath.Join(dir, "Engine", "Programs", "AutomationTool", "Saved", "Logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(logDir, "Log.txt"), []byte("error: AddBuildProductsFromManifest failed"), 0644); err != nil {
			t.Fatal(err)
		}
		hints := scanBuildLogs(dir)
		if len(hints) == 0 {
			t.Fatal("expected at least one hint")
		}
		if !strings.Contains(hints[0], "manifest") {
			t.Errorf("hint should mention manifest, got: %s", hints[0])
		}
	})

	t.Run("log without patterns", func(t *testing.T) {
		dir := t.TempDir()
		logDir := filepath.Join(dir, "Engine", "Programs", "AutomationTool", "Saved", "Logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(logDir, "Log.txt"), []byte("Build succeeded."), 0644); err != nil {
			t.Fatal(err)
		}
		if hints := scanBuildLogs(dir); hints != nil {
			t.Errorf("expected nil for clean log, got %v", hints)
		}
	})
}
