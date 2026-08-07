package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisableDumpSyms(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	configPath := filepath.Join(appData, "Unreal Engine", "UnrealBuildTool", "BuildConfiguration.xml")
	original := "<Configuration>\n</Configuration>\n"
	writeTestFile(t, configPath, original)

	restore := newTestBuilder(BuildOptions{}).disableDumpSyms()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<bDisableDumpSyms>true</bDisableDumpSyms>") {
		t.Fatalf("patched config = %s", data)
	}
	restore()
	assertFileEquals(t, configPath, original)
}

func TestDisableDumpSymsMissingConfig(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	newTestBuilder(BuildOptions{}).disableDumpSyms()()
}

func TestRestoreFileWriteError(t *testing.T) {
	// restoreFile's closure surfaces (logs) a WriteFile failure without panicking.
	path := filepath.Join(t.TempDir(), "missing-dir", "config.xml")
	restore := restoreFile(path, []byte("original"))
	restore()
}

func TestDisableDumpSymsInConfigWriteError(t *testing.T) {
	// Put a directory at the config path: patch content differs from the
	// original, so WriteFile is attempted and fails against the directory.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.xml")
	if err := os.Mkdir(configPath, 0755); err != nil {
		t.Fatal(err)
	}

	restore := newTestBuilder(BuildOptions{}).disableDumpSymsInConfig(configPath, []byte("<Configuration>\n</Configuration>\n"))
	restore()
}
