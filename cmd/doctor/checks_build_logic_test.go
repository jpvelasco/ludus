package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestCheckStaleBuildArtifacts(t *testing.T) {
	t.Run("no engine source skips", func(t *testing.T) {
		d := checkStaleBuildArtifacts(&config.Config{})
		if d.status != "ok" {
			t.Errorf("status = %q, want ok", d.status)
		}
	})

	t.Run("no build found is clean", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Engine.SourcePath = t.TempDir() // no editor binary inside
		d := checkStaleBuildArtifacts(cfg)
		if d.status != "ok" {
			t.Errorf("status = %q, want ok", d.status)
		}
	})

	t.Run("recent build is ok", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Engine.SourcePath = t.TempDir()
		writeEditorBinary(t, cfg.Engine.SourcePath)
		d := checkStaleBuildArtifacts(cfg)
		if d.status != "ok" {
			t.Errorf("status = %q, want ok (recent build)", d.status)
		}
	})

	t.Run("old build warns", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Engine.SourcePath = t.TempDir()
		path := writeEditorBinary(t, cfg.Engine.SourcePath)
		old := time.Now().Add(-40 * 24 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		d := checkStaleBuildArtifacts(cfg)
		if d.status != "warn" {
			t.Errorf("status = %q, want warn (40-day-old build)", d.status)
		}
	})
}

// writeEditorBinary creates the platform editor binary under engineSrc and
// returns its path.
func writeEditorBinary(t *testing.T, engineSrc string) string {
	t.Helper()
	var rel string
	if runtime.GOOS == "windows" {
		rel = filepath.Join("Engine", "Binaries", "Win64", "UnrealEditor.exe")
	} else {
		rel = filepath.Join("Engine", "Binaries", "Linux", "UnrealEditor")
	}
	path := filepath.Join(engineSrc, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckBuildState_CleanWhenNoState(t *testing.T) {
	t.Chdir(t.TempDir()) // no .ludus/state.json
	d := checkBuildState()
	if d.status != "ok" {
		t.Errorf("status = %q, want ok for empty state", d.status)
	}
}

func TestCheckCacheIntegrity(t *testing.T) {
	t.Run("missing cache is ok", func(t *testing.T) {
		t.Chdir(t.TempDir())
		d := checkCacheIntegrity()
		if d.status != "ok" {
			t.Errorf("status = %q, want ok for absent cache", d.status)
		}
	})

	t.Run("corrupt cache warns", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".ludus", "cache.json"), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		d := checkCacheIntegrity()
		if d.status != "warn" {
			t.Errorf("status = %q, want warn for corrupt cache", d.status)
		}
	})
}
