package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

// withScannerInput swaps the package scanner to read from in for the duration of
// the test, restoring the original afterward. Lets prompt-driven paths be tested
// without real stdin.
func withScannerInput(t *testing.T, in string) {
	t.Helper()
	orig := scanner
	scanner = bufio.NewScanner(strings.NewReader(in))
	t.Cleanup(func() { scanner = orig })
}

// writeBuildVersion creates an engine tree with a Build.version file under root.
func writeBuildVersion(t *testing.T, root string, major, minor, patch int) {
	t.Helper()
	dir := filepath.Join(root, "Engine", "Build")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`{"MajorVersion":%d,"MinorVersion":%d,"PatchVersion":%d}`, major, minor, patch)
	if err := os.WriteFile(filepath.Join(dir, "Build.version"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEngineSetupFile(t *testing.T) {
	// Returns the OS-appropriate Setup script name; just assert it's non-empty
	// and one of the two valid values.
	got := engineSetupFile()
	if got != "Setup.sh" && got != "Setup.bat" {
		t.Errorf("engineSetupFile() = %q, want Setup.sh or Setup.bat", got)
	}
}

func TestIsEngineSourceDir(t *testing.T) {
	t.Run("with setup file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, engineSetupFile()), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if !isEngineSourceDir(dir) {
			t.Error("expected true when setup file present")
		}
	})
	t.Run("without setup file", func(t *testing.T) {
		if isEngineSourceDir(t.TempDir()) {
			t.Error("expected false when setup file absent")
		}
	})
}

func TestScanGlob(t *testing.T) {
	root := t.TempDir()
	// Two matching dirs, one non-matching dir, one matching FILE (must be skipped).
	for _, d := range []string{"UnrealEngine", "UnrealEngine5", "OtherProject"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "UnrealEngineFile"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	var found []string
	scanGlob(root, "UnrealEngine*", func(p string) { found = append(found, filepath.Base(p)) })

	if len(found) != 2 {
		t.Fatalf("expected 2 matching dirs, got %d: %v", len(found), found)
	}
	for _, f := range found {
		if f == "UnrealEngineFile" {
			t.Error("scanGlob should skip files, only emit directories")
		}
	}
}

func TestEngineVersionLabel(t *testing.T) {
	t.Run("valid build version", func(t *testing.T) {
		dir := t.TempDir()
		writeBuildVersion(t, dir, 5, 7, 3)
		if got := engineVersionLabel(dir); got != " (v5.7.3)" {
			t.Errorf("engineVersionLabel() = %q, want ' (v5.7.3)'", got)
		}
	})
	t.Run("no build version", func(t *testing.T) {
		if got := engineVersionLabel(t.TempDir()); got != "" {
			t.Errorf("expected empty label, got %q", got)
		}
	})
}

func TestDetectEngineVersion(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		if got := detectEngineVersion(""); got != "" {
			t.Errorf("expected empty for empty path, got %q", got)
		}
	})
	t.Run("auto-detect from build version", func(t *testing.T) {
		dir := t.TempDir()
		writeBuildVersion(t, dir, 5, 6, 1)
		if got := detectEngineVersion(dir); got != "5.6.1" {
			t.Errorf("detectEngineVersion() = %q, want 5.6.1", got)
		}
	})
	t.Run("falls back to prompt when Build.version unparseable", func(t *testing.T) {
		// No Build.version under the path → ParseBuildVersion fails → prompt.
		// Inject the answer so the interactive fallback is exercised.
		withScannerInput(t, "5.8.0\n")
		if got := detectEngineVersion(t.TempDir()); got != "5.8.0" {
			t.Errorf("detectEngineVersion() = %q, want 5.8.0 (from prompt)", got)
		}
	})
}

func TestDeployTargetDefault(t *testing.T) {
	targets := []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
	t.Run("nil config returns 0", func(t *testing.T) {
		if got := deployTargetDefault(nil, targets); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
	t.Run("empty target returns 0", func(t *testing.T) {
		if got := deployTargetDefault(&config.Config{}, targets); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
	t.Run("matches existing target index", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Deploy.Target = "ec2"
		if got := deployTargetDefault(cfg, targets); got != 2 {
			t.Errorf("got %d, want 2 (ec2)", got)
		}
	})
	t.Run("unknown target returns 0", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Deploy.Target = "nonsense"
		if got := deployTargetDefault(cfg, targets); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
}

func TestExistingString(t *testing.T) {
	getter := func(c *config.Config) string { return c.AWS.Region }

	t.Run("nil existing returns default", func(t *testing.T) {
		if got := existingString("us-east-1", nil, getter); got != "us-east-1" {
			t.Errorf("got %q, want default", got)
		}
	})
	t.Run("empty value returns default", func(t *testing.T) {
		if got := existingString("us-east-1", &config.Config{}, getter); got != "us-east-1" {
			t.Errorf("got %q, want default", got)
		}
	})
	t.Run("existing value wins", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.AWS.Region = "eu-west-1"
		if got := existingString("us-east-1", cfg, getter); got != "eu-west-1" {
			t.Errorf("got %q, want eu-west-1", got)
		}
	})
}
