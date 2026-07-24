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
