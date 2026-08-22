//go:build windows

package game

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

// writeSnapshotRunUAT overwrites RunUAT.bat with a stub that captures
// BuildConfiguration.xml exactly when UAT would execute.
func writeSnapshotRunUAT(t *testing.T, engineRoot string) {
	t.Helper()
	batch := filepath.Join(engineRoot, "Engine", "Build", "BatchFiles")
	script := "@echo off\r\n" +
		"type \"%APPDATA%\\Unreal Engine\\UnrealBuildTool\\BuildConfiguration.xml\" > \"%LUDUS_TEST_CONFIG_SNAPSHOT%\"\r\n" +
		"exit /b 0\r\n"
	writeTestFile(t, filepath.Join(batch, "RunUAT.bat"), script)
}

// TestBuildArm64PatchActiveDuringUAT verifies the dump_syms workaround is in
// effect while UAT executes (the failure mode it exists to prevent happens
// during the build, not during environment preparation) and restored once
// Build completes.
func TestBuildArm64PatchActiveDuringUAT(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	configPath := filepath.Join(appdata, "Unreal Engine", "UnrealBuildTool", "BuildConfiguration.xml")
	const original = "<BuildConfiguration>\n</BuildConfiguration>\n"
	writeTestFile(t, configPath, original)

	snapshot := filepath.Join(t.TempDir(), "during-build.xml")
	t.Setenv("LUDUS_TEST_CONFIG_SNAPSHOT", snapshot)

	opts := flowBaseOptions(t)
	opts.Arch = "arm64"
	writeSnapshotRunUAT(t, opts.EnginePath)

	b := NewBuilder(opts, runner.NewRunner(false, false))
	result, err := b.Build(context.Background())
	if err != nil || !result.Success {
		t.Fatalf("Build(arm64) = (%v, %v), want success", result, err)
	}

	during, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatalf("reading during-build snapshot: %v", err)
	}
	if !strings.Contains(string(during), "<bDisableDumpSyms>true</bDisableDumpSyms>") {
		t.Errorf("dump_syms not disabled during UAT; snapshot = %q", during)
	}

	assertFileEquals(t, configPath, original)
}
