package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestCheckToolchainConsistency(t *testing.T) {
	t.Run("no engine source", func(t *testing.T) {
		assertDiagnostic(t, checkToolchainConsistency(&config.Config{}), "ok", "skipped")
	})
	t.Run("unknown engine version", func(t *testing.T) {
		cfg := engineConfig(t, "5.3.2")
		assertDiagnostic(t, checkToolchainConsistency(cfg), "ok", "no known")
	})
	t.Run("required toolchain missing", func(t *testing.T) {
		cfg := engineConfig(t, "5.6.1")
		t.Setenv("LINUX_MULTIARCH_ROOT", t.TempDir())
		assertDiagnostic(t, checkToolchainConsistency(cfg), "warn", "v25 not found")
	})
	t.Run("environment points to wrong version", func(t *testing.T) {
		cfg := engineConfig(t, "5.6.1")
		root := createToolchain(t, "v25_clang-18.1")
		t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(root, "v24_clang-17"))
		assertDiagnostic(t, checkToolchainConsistency(cfg), "warn", "points to")
	})
	t.Run("required toolchain matches", func(t *testing.T) {
		cfg := engineConfig(t, "5.6.1")
		root := createToolchain(t, "v25_clang-18.1")
		t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(root, "v25_clang-18.1"))
		assertDiagnostic(t, checkToolchainConsistency(cfg), "ok", "matches")
	})
}

func TestCheckDiskSpaceClassifiesResult(t *testing.T) {
	cfg := &config.Config{Engine: config.EngineConfig{SourcePath: t.TempDir()}}
	got := checkDiskSpace(cfg)
	if got.name != "Disk Space" {
		t.Fatalf("name = %q, want Disk Space", got.name)
	}
	valid := map[string]bool{"ok": true, "warn": true, "fail": true}
	if !valid[got.status] || got.message == "" {
		t.Fatalf("unexpected diagnostic: %+v", got)
	}
}

func TestCheckDiskSpaceInvalidPath(t *testing.T) {
	cfg := &config.Config{Engine: config.EngineConfig{SourcePath: filepath.Join(t.TempDir(), "missing")}}
	assertDiagnostic(t, checkDiskSpace(cfg), "ok", "could not determine")
}

func TestCheckGitState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	t.Run("outside repository", func(t *testing.T) {
		t.Chdir(t.TempDir())
		assertDiagnostic(t, checkGitState(), "ok", "not in a git repository")
	})
	t.Run("clean repository", func(t *testing.T) {
		repo := initGitRepository(t)
		t.Chdir(repo)
		assertDiagnostic(t, checkGitState(), "ok", "working tree clean")
	})
	t.Run("counts changed files", func(t *testing.T) {
		repo := initGitRepository(t)
		writeTestFile(t, filepath.Join(repo, "one.txt"), "one")
		writeTestFile(t, filepath.Join(repo, "two.txt"), "two")
		t.Chdir(repo)
		assertDiagnostic(t, checkGitState(), "ok", "2 modified file(s)")
	})
}

func TestDockerNotInstalledDiagnostic(t *testing.T) {
	got := dockerNotInstalledDiagnostic()
	if runtime.GOOS == "windows" {
		assertDiagnostic(t, got, "ok", "not needed")
		return
	}
	assertDiagnostic(t, got, "warn", "not installed")
}

func engineConfig(t *testing.T, version string) *config.Config {
	t.Helper()
	return &config.Config{Engine: config.EngineConfig{SourcePath: t.TempDir(), Version: version}}
}

func createToolchain(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func initGitRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return repo
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertDiagnostic(t *testing.T, got diagnostic, status, message string) {
	t.Helper()
	if got.status != status || !strings.Contains(got.message, message) {
		t.Errorf("diagnostic = %+v, want status %q containing %q", got, status, message)
	}
}
