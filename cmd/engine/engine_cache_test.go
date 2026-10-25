package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// resetEngineFlags saves and clears the package-level engine flags around a
// test, restoring them on cleanup.
func resetEngineFlags(t *testing.T) {
	t.Helper()
	oldPath, oldBackend, oldNoCache := uePath, backend, noCache
	uePath, backend, noCache = "", "", false
	t.Cleanup(func() { uePath, backend, noCache = oldPath, oldBackend, oldNoCache })
}

// seedEngineHostBinaries creates the host Binaries directory a completed
// native engine build produces, so a warm StageEngine entry passes its output
// validation (#739).
func seedEngineHostBinaries(t *testing.T, engineRoot string) {
	t.Helper()
	binDir := filepath.Join(engineRoot, "Engine", "Binaries", hostBinariesDirForTest())
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// hostBinariesDirForTest mirrors globals.EngineBuildOutput's host directory.
func hostBinariesDirForTest() string {
	switch runtime.GOOS {
	case "windows":
		return "Win64"
	case "darwin":
		return "Mac"
	default:
		return "Linux"
	}
}

// gitEngineTree creates a fresh engine tree that is its own git repository
// with one commit, so two trees hash to different git HEADs — the engine
// cache key's identity for source content. marker distinguishes the commits.
func gitEngineTree(t *testing.T, marker string) string {
	t.Helper()
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init", "--quiet"},
		{"config", "user.email", "test@ludus.local"},
		{"config", "user.name", "ludus-test"},
		{"add", "-A"},
		{"commit", "--quiet", "-m", "engine tree"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return root
}

// TestResolvedEngineConfigAppliesOverrides pins #729's resolution contract:
// the resolved config carries the effective --path and --backend, so
// validation, hashing, and builders all see the same values.
func TestResolvedEngineConfigAppliesOverrides(t *testing.T) {
	override := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "/configured/absent", Version: "5.7.3", Backend: "native"},
		Game:   config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg)
	resetEngineFlags(t)

	uePath, backend = override, "podman"
	resolved := resolvedEngineConfig()
	if resolved.Engine.SourcePath != override {
		t.Errorf("SourcePath = %q, want the --path override %q", resolved.Engine.SourcePath, override)
	}
	if resolved.Engine.Backend != "podman" {
		t.Errorf("Backend = %q, want podman", resolved.Engine.Backend)
	}

	uePath, backend = "", ""
	resolved = resolvedEngineConfig()
	if resolved.Engine.SourcePath != "/configured/absent" || resolved.Engine.Backend != "native" {
		t.Errorf("resolved config = %+v, want the configured values with no flags", resolved.Engine)
	}
}

// TestRunBuildPathOverrideValidatesOverridePath pins the validation half of
// #729: an absent configured source path must not fail the build when --path
// points at a valid engine tree.
func TestRunBuildPathOverrideValidatesOverridePath(t *testing.T) {
	override := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: filepath.Join(t.TempDir(), "absent"),
			Version:    "5.7.3",
			Backend:    "native",
			MaxJobs:    1,
		},
		Game: config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	resetEngineFlags(t)
	uePath = override

	if err := runBuild(engineCmd(t), nil); err != nil {
		t.Fatalf("runBuild() error = %v, want nil with a valid --path override", err)
	}
}

// TestRunNativeEngineBuildPathOverrideChangesCacheKey pins the cache half of
// #729: a warm entry for one --path checkout must not satisfy a request for a
// different checkout.
func TestRunNativeEngineBuildPathOverrideChangesCacheKey(t *testing.T) {
	first := gitEngineTree(t, "first-checkout")
	second := gitEngineTree(t, "second-checkout")
	seedEngineHostBinaries(t, second)

	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: first, Version: "5.7.3", Backend: "native"},
		Game:   config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	resetEngineFlags(t)

	// Warm the cache under the first checkout...
	seedEngineCache(t, cache.EngineKey(cfg))
	// ...then point --path at the second one.
	uePath = second

	output := captureStdout(func() {
		if err := runNativeEngineBuild(engineCmd(t), resolvedEngineConfig()); err != nil {
			t.Fatalf("runNativeEngineBuild() error = %v, want nil", err)
		}
	})
	if strings.Contains(output, "(cached), skipping") {
		t.Errorf("second checkout reused the first checkout's cache entry: %s", output)
	}
}

// TestRunNativeEngineBuildCacheHitIgnoresMissingBinaries pins #739 for the
// native engine stage: a warm entry whose built engine binaries are gone must
// be ignored and the build rerun.
func TestRunNativeEngineBuildCacheHitIgnoresMissingBinaries(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineRoot, Version: "5.7.3", Backend: "native", MaxJobs: 1},
		Game:   config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	resetEngineFlags(t)

	seedEngineCache(t, cache.EngineKey(cfg))
	// No Engine/Binaries/<host> directory: the hit must be discarded (#739).

	output := captureStdout(func() {
		if err := runNativeEngineBuild(engineCmd(t), cfg); err != nil {
			t.Fatalf("runNativeEngineBuild() error = %v, want nil", err)
		}
	})
	if strings.Contains(output, "(cached), skipping") {
		t.Errorf("cache hit honored with missing engine binaries: %s", output)
	}
	if !strings.Contains(output, "cache hit ignored: output missing") {
		t.Errorf("output = %q, want the ignored-hit notice", output)
	}
}

// TestRunNativeEngineBuildCacheHitHonoredWithBinaries pins #739's positive
// path: a warm entry whose host Binaries directory exists skips the build.
func TestRunNativeEngineBuildCacheHitHonoredWithBinaries(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	seedEngineHostBinaries(t, engineRoot)
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineRoot, Version: "5.7.3", Backend: "native"},
		Game:   config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	resetEngineFlags(t)

	seedEngineCache(t, cache.EngineKey(cfg))

	output := captureStdout(func() {
		if err := runNativeEngineBuild(engineCmd(t), cfg); err != nil {
			t.Fatalf("runNativeEngineBuild() error = %v, want nil", err)
		}
	})
	if !strings.Contains(output, "(cached), skipping") {
		t.Errorf("output = %q, want the cache hit to skip the build", output)
	}
}
