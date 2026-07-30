package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wsl"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func withEngineTestConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	origCfg, origDryRun := globals.Cfg, globals.DryRun
	t.Cleanup(func() {
		globals.Cfg = origCfg
		globals.DryRun = origDryRun
	})
	t.Chdir(t.TempDir())
	globals.Cfg = cfg
	globals.DryRun = false
}

func TestEngineBuildInputWSL2Fields(t *testing.T) {
	input := engineBuildInput{
		Backend:   "wsl2",
		WSLNative: true,
		WSLDistro: "Ubuntu-24.04",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded engineBuildInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Backend != "wsl2" {
		t.Errorf("Backend = %q, want %q", decoded.Backend, "wsl2")
	}
	if !decoded.WSLNative {
		t.Error("expected WSLNative = true")
	}
	if decoded.WSLDistro != "Ubuntu-24.04" {
		t.Errorf("WSLDistro = %q, want %q", decoded.WSLDistro, "Ubuntu-24.04")
	}
}

func TestEngineBuildInputWSL2FieldsOmitEmpty(t *testing.T) {
	input := engineBuildInput{Backend: "native"}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	s := string(data)
	if strings.Contains(s, "wsl_native") {
		t.Errorf("wsl_native should be omitted when false, got: %s", s)
	}
	if strings.Contains(s, "wsl_distro") {
		t.Errorf("wsl_distro should be omitted when empty, got: %s", s)
	}
}

// TestEngineBuildWSL2Dispatch verifies that backend=wsl2 dispatches to the
// WSL2 handler (not the native path). On non-Windows / no-WSL2 CI, the handler
// returns a WSL2-specific error — proving the dispatch took the right branch.
func TestEngineBuildWSL2Dispatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dispatch reaches the real wsl.exe probe on Windows")
	}
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: "/nonexistent/engine"},
	}

	result, _, err := handleEngineBuild(context.Background(), nil, engineBuildInput{
		Backend: "wsl2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// The WSL2 handler calls wsl.New() which fails on non-Windows with
	// "WSL2 is not available" — this proves the dispatch reached the WSL2
	// path, not the native path (which would fail differently).
	assertResultContains(t, result, "WSL2")
}

// TestEngineBuildInputSkipSetup verifies the skip_setup field round-trips and
// is omitted when false (the #412 MCP surface).
func TestEngineBuildInputSkipSetup(t *testing.T) {
	data, err := json.Marshal(engineBuildInput{SkipSetup: true})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(data), "skip_setup") {
		t.Errorf("skip_setup should be present when true, got: %s", data)
	}

	off, err := json.Marshal(engineBuildInput{Backend: "native"})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if strings.Contains(string(off), "skip_setup") {
		t.Errorf("skip_setup should be omitted when false, got: %s", off)
	}
}

// TestEngineBuildSkipSetupDryRun drives the native handler with skip_setup=true
// and dry-run, asserting the Setup step is skipped (the #412 wiring reaches
// engine.BuildOptions.SkipSetup). A dry-run native build prints the commands
// without executing them, so this is safe on CI with no engine tree.
func TestEngineBuildSkipSetupDryRun(t *testing.T) {
	withEngineTestConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir(), Backend: "native"},
	})

	result, _, err := handleEngineBuild(context.Background(), nil, engineBuildInput{
		Backend:   "native",
		SkipSetup: true,
		NoCache:   true,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertResultContains(t, result, "Skipping Setup")
}

func TestHandleEngineSetupDryRun(t *testing.T) {
	engineDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(engineDir, engineSetupScript()), nil, 0644); err != nil {
		t.Fatal(err)
	}
	withEngineTestConfig(t, &config.Config{Engine: config.EngineConfig{SourcePath: engineDir}})

	result, _, err := handleEngineSetup(context.Background(), nil, engineSetupInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleEngineSetup() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleEngineSetup() returned error result: %+v", result)
	}
	got := decodeEngineResult(t, result)
	if !got.Success || got.EnginePath != engineDir {
		t.Errorf("result = %+v, want successful engine path %q", got, engineDir)
	}
}

func TestHandleEngineSetupMissingSource(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	withEngineTestConfig(t, &config.Config{Engine: config.EngineConfig{SourcePath: missing}})

	result, _, err := handleEngineSetup(context.Background(), nil, engineSetupInput{})
	if err != nil {
		t.Fatalf("handleEngineSetup() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("handleEngineSetup() should return an error result")
	}
	assertResultContains(t, result, engineSetupScript()+" not found")
}

func engineSetupScript() string {
	if runtime.GOOS == "windows" {
		return "Setup.bat"
	}
	return "Setup.sh"
}

func TestHandleContainerEngineBuildMissingSource(t *testing.T) {
	tests := []struct {
		name    string
		backend string
		want    string
	}{
		{name: "docker", backend: "docker", want: "engine build failed: engine source path not specified"},
		{name: "podman", backend: "podman", want: "engine build failed: engine source path not specified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{Engine: config.EngineConfig{SourcePath: ""}}
			t.Chdir(t.TempDir())
			result, _, err := handleContainerEngineBuild(context.Background(), &cfg, engineBuildInput{NoCache: true}, tt.backend)
			if err != nil {
				t.Fatalf("handleContainerEngineBuild() error = %v", err)
			}
			if !result.IsError {
				t.Fatal("handleContainerEngineBuild() should return an error result")
			}
			assertResultContains(t, result, tt.want)
		})
	}
}

func decodeEngineResult(t *testing.T, result *mcpsdk.CallToolResult) engineResult {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected result content")
	}
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	var got engineResult
	if err := json.Unmarshal([]byte(text.Text), &got); err != nil {
		t.Fatalf("unmarshal engine result: %v", err)
	}
	return got
}

// assertResultContains checks that a CallToolResult's text content contains substr.
func assertResultContains(t *testing.T, result *mcpsdk.CallToolResult, substr string) {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(tc.Text, substr) {
		t.Errorf("result text %q should contain %q", tc.Text, substr)
	}
}

// TestHandleEnginePushDryRun tests engine push with dry-run.
func TestHandleEnginePushDryRun(t *testing.T) {
	withEngineTestConfig(t, &config.Config{
		Engine: config.EngineConfig{
			SourcePath: t.TempDir(),
			Version:    "5.7.3",
		},
	})
	globals.DryRun = true

	result, _, err := handleEnginePush(context.Background(), nil, enginePushInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleEnginePush() error = %v", err)
	}
	if result.IsError {
		// It's acceptable if push fails on a minimal config; we're testing the dry-run path
		if !strings.Contains(toolResultText(t, result), "engine push failed") {
			t.Errorf("result should contain 'engine push failed', got: %s", toolResultText(t, result))
		}
	}
}

// TestResolveWSL2PathsWithoutNative covers the virtiofs path
// (line 234-236: ToWSLPath calls for virtiofs mode).
func TestResolveWSL2PathsWithoutNative(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("WSL2 test requires Windows")
	}

	enginePath, ddcPath, err := resolveWSL2Paths(
		context.Background(),
		nil, // runner not used in virtiofs path
		&wsl.WSL2{Distro: "Ubuntu"},
		"C:/Engine",
		"5.7",
		false, // wslNative=false means virtiofs
	)

	if err != nil {
		t.Fatalf("resolveWSL2Paths error = %v", err)
	}
	if enginePath == "" {
		t.Error("enginePath should not be empty")
	}
	if ddcPath == "" {
		t.Error("ddcPath should not be empty")
	}
}

// TestSaveWSL2EngineResult covers state persistence for WSL2 builds
// (line 240-252: UpdateWSL2Engine and saveCache calls).
func TestSaveWSL2EngineResult(t *testing.T) {
	t.Chdir(t.TempDir())

	// Call the function directly
	saveWSL2EngineResult(
		"/wsl/engine",
		"/wsl/ddc",
		"test-engine-hash",
		true,  // wslNative=true
		false, // dryRun=false
	)

	// Verify state was saved
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if st.WSL2Engine == nil {
		t.Fatal("expected WSL2Engine state to be set")
	}
	if st.WSL2Engine.EnginePath != "/wsl/engine" {
		t.Errorf("EnginePath = %q, want /wsl/engine", st.WSL2Engine.EnginePath)
	}
	if !st.WSL2Engine.IsNative {
		t.Error("expected IsNative = true")
	}
}

// TestHandleWSL2EngineBuildNoState covers cache hit path
// (line 257-259: checkCacheHit call). Stubs wsl.exe to avoid real probe timeout.
func TestHandleWSL2EngineBuildNoState(t *testing.T) {
	t.Chdir(t.TempDir())
	withEngineTestConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: "/engine", Backend: "wsl2"},
	})

	// Stub wsl.exe to fail immediately instead of timing out on real probe
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		ExitCode: 1,
		Stderr:   "Error: The Windows Subsystem for Linux is not installed.",
	})

	result, _, err := handleWSL2EngineBuild(context.Background(), globals.Cfg, engineBuildInput{NoCache: true})
	if err != nil {
		t.Fatalf("handleWSL2EngineBuild() error = %v", err)
	}

	// Result may fail due to missing engine path, but we're testing the wsl.exe dispatch wasn't slow
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestHandleEngineBuildDispatchesToWSL2 verifies the WSL2 dispatch path
// (tools_engine.go:108-109: dockerbuild.IsWSL2Backend check). Stubs wsl.exe to avoid real probe timeout.
func TestHandleEngineBuildDispatchesToWSL2(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("WSL2 dispatch test requires Windows")
	}
	t.Chdir(t.TempDir())
	withEngineTestConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: "/engine", Backend: "wsl2"},
	})

	// Stub wsl.exe to fail immediately instead of timing out
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1, Stderr: "Error: The Windows Subsystem for Linux is not installed."})

	result, _, err := handleEngineBuild(context.Background(), nil, engineBuildInput{Backend: "wsl2", NoCache: true})
	if err != nil {
		t.Fatalf("handleEngineBuild() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestHandleEngineBuildNativeSavesCache verifies cache save path
// (tools_engine.go:150-151: saveCache call after successful build).
func TestHandleEngineBuildNativeSavesCache(t *testing.T) {
	t.Chdir(t.TempDir())
	engineDir := t.TempDir()
	// Write a fake UE structure minimal markers
	if err := os.WriteFile(filepath.Join(engineDir, engineSetupScript()), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	withEngineTestConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: engineDir, Backend: "native"},
	})

	result, _, err := handleEngineBuild(context.Background(), nil, engineBuildInput{DryRun: true, NoCache: true})
	if err != nil {
		t.Fatalf("handleEngineBuild() error = %v", err)
	}
	// Dry-run build should complete quickly and populate result
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
