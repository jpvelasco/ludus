package pipeline

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// captureStdout captures stdout during fn execution and returns the captured output.
func captureStdout(fn func()) string {
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		return ""
	}
	defer func() { _ = r.Close() }()

	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan bool, 1)
	go func() {
		_, _ = io.Copy(&buf, r)
		done <- true
	}()

	fn()

	_ = w.Close()
	<-done

	return buf.String()
}

// testContextOpts controls variations in test pipelineCtx setup.
type testContextOpts struct {
	engineVersion    string
	containerBackend string
	fullVersion      string
	ddcMode          string
	ddcPath          string
	ddcZenPath       string
	arch             string
	withRecordingR   bool
	withBuildCache   bool
	engineHash       string
	serverHash       string
	clientHash       string
}

// newTestPipelineCtx constructs a *pipelineCtx with sensible defaults.
// Call sites can override fields as needed. withRecordingR=true captures runner output.
func newTestPipelineCtx(t *testing.T, cfg *config.Config, opts *testContextOpts) *pipelineCtx {
	if opts == nil {
		opts = &testContextOpts{}
	}

	// Defaults
	if opts.engineVersion == "" {
		opts.engineVersion = "5.7"
	}
	if opts.arch == "" {
		opts.arch = "amd64"
	}
	if opts.ddcMode == "" {
		opts.ddcMode = "none"
	}
	if opts.fullVersion == "" {
		opts.fullVersion = "5.7.3"
	}

	var r *runner.Runner
	if opts.withRecordingR {
		r, _ = testsupport.RecordingRunner()
	} else {
		r = globals.NewRunner()
	}

	var bc *cache.Cache
	if opts.withBuildCache {
		bc = newTestCache()
	}

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		engineVersion:    opts.engineVersion,
		containerBackend: opts.containerBackend,
		fullVersion:      opts.fullVersion,
		ddcMode:          opts.ddcMode,
		ddcPath:          opts.ddcPath,
		ddcZenPath:       opts.ddcZenPath,
		arch:             opts.arch,
		engineHash:       opts.engineHash,
		serverHash:       opts.serverHash,
		clientHash:       opts.clientHash,
		buildCache:       bc,
		target:           &stubTarget{},
	}

	return p
}

// newTestCache constructs a fresh cache for testing.
func newTestCache() *cache.Cache {
	return &cache.Cache{Entries: make(map[cache.StageKey]*cache.Entry)}
}

// stubCrossCompileToolchain points LINUX_MULTIARCH_ROOT at a directory holding
// the toolchain the configured engine version expects.
//
// This is a safety requirement, not a convenience: stageValidate builds its
// prereq.Checker with fix=true, and on a Windows host a *missing* toolchain sends
// toolchainNotFoundResult into fixCrossCompileToolchain, which downloads an
// installer and launches it via elevated PowerShell (`Start-Process -Verb RunAs
// -Wait`). In CI that blocks forever — it hung the Windows test leg until the
// 10-minute panic. Making the check succeed keeps that branch unreachable.
func stubCrossCompileToolchain(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "v26_clang-20.1.8-rockylinux8"), 0o755); err != nil {
		t.Fatalf("create toolchain dir: %v", err)
	}
	t.Setenv("LINUX_MULTIARCH_ROOT", root)
}

// setupTestContext sets up a complete test environment: FakeEngineTree, config,
// globals, and a stub target. Returns (engineRoot, projectPath, cfg).
// Call globals.SetGlobals separately if needed for dry-run or other flags.
func setupTestContext(t *testing.T, projectName string) (string, string, *config.Config) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"), testsupport.WithLinuxToolchain("v26_clang-20.1.8-rockylinux8"))
	projectPath := testsupport.FakeProject(t, projectName)
	stubCrossCompileToolchain(t)

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: projectName,
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	return engineRoot, projectPath, cfg
}
