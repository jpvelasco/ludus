package testsupport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TreeOption configures FakeEngineTree behavior.
type TreeOption func(*treeConfig)

// treeConfig holds the state for FakeEngineTree options.
type treeConfig struct {
	version        string
	linuxToolchain string
	skipSetup      bool
}

// WithVersion sets the Build.version contents (e.g. "5.7.4").
func WithVersion(v string) TreeOption {
	return func(tc *treeConfig) {
		tc.version = v
	}
}

// WithoutSetup omits Setup.bat and Setup.sh.
func WithoutSetup() TreeOption {
	return func(tc *treeConfig) {
		tc.skipSetup = true
	}
}

// WithLinuxToolchain creates a toolchain marker directory.
// v is the toolchain version string (e.g. "v26_clang-20.1.8-rockylinux8").
func WithLinuxToolchain(v string) TreeOption {
	return func(tc *treeConfig) {
		tc.linuxToolchain = v
	}
}

// FakeEngineTree creates a minimal UE5 engine source layout under t.TempDir()
// and returns its root. Sufficient to drive engine/game builders in dry-run.
func FakeEngineTree(t *testing.T, opts ...TreeOption) string {
	t.Helper()

	cfg := &treeConfig{version: "5.7.3"}
	for _, opt := range opts {
		opt(cfg)
	}

	root := t.TempDir()
	createTreeScripts(t, root, cfg)
	createTreeBuildVersion(t, root, cfg)
	createTreeToolchain(t, root, cfg)

	return root
}

// createTreeScripts creates Setup and GenerateProjectFiles scripts.
func createTreeScripts(t *testing.T, root string, cfg *treeConfig) {
	t.Helper()

	if !cfg.skipSetup {
		writeFile(t, filepath.Join(root, "Setup.bat"), "setup_bat")
		writeFile(t, filepath.Join(root, "Setup.sh"), "setup_sh")
	}

	writeFile(t, filepath.Join(root, "GenerateProjectFiles.bat"), "generate_bat")
	writeFile(t, filepath.Join(root, "GenerateProjectFiles.sh"), "generate_sh")

	batchDir := filepath.Join(root, "Engine", "Build", "BatchFiles")
	if err := os.MkdirAll(batchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(batchDir, "Build.bat"), "build_bat")
	writeFile(t, filepath.Join(batchDir, "Build.sh"), "build_sh")
	writeFile(t, filepath.Join(batchDir, "RunUAT.bat"), "runuat_bat")
	writeFile(t, filepath.Join(batchDir, "RunUAT.sh"), "runuat_sh")
}

// createTreeBuildVersion creates the Build.version JSON file.
func createTreeBuildVersion(t *testing.T, root string, cfg *treeConfig) {
	t.Helper()

	buildVersionDir := filepath.Join(root, "Engine", "Build")
	versionParts := parseVersion(cfg.version)
	buildVersionData := map[string]int{
		"MajorVersion": versionParts.major,
		"MinorVersion": versionParts.minor,
		"PatchVersion": versionParts.patch,
	}
	versionJSON, err := json.Marshal(buildVersionData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildVersionDir, "Build.version"), versionJSON, 0o644); err != nil {
		t.Fatal(err)
	}
}

// createTreeToolchain creates the Linux toolchain marker directory if requested.
func createTreeToolchain(t *testing.T, root string, cfg *treeConfig) {
	t.Helper()

	if cfg.linuxToolchain == "" {
		return
	}

	toolchainDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", cfg.linuxToolchain)
	if err := os.MkdirAll(toolchainDir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeFile is a test helper to write a file.
func writeFile(t *testing.T, path, kind string) {
	t.Helper()

	content := fileContent(kind)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fileContent returns stub file contents by kind.
func fileContent(kind string) string {
	stubs := map[string]string{
		"setup_bat":    "@echo off\nREM Setup batch file\n",
		"setup_sh":     "#!/bin/sh\n# Setup shell script\n",
		"generate_bat": "@echo off\nREM Generate project files\n",
		"generate_sh":  "#!/bin/sh\n# Generate project files\n",
		"build_bat":    "@echo off\nREM Build batch file\n",
		"build_sh":     "#!/bin/sh\n# Build shell script\n",
		"runuat_bat":   "@echo off\nREM RunUAT batch file\n",
		"runuat_sh":    "#!/bin/sh\n# RunUAT shell script\n",
	}
	if content, ok := stubs[kind]; ok {
		return content
	}
	return ""
}

// versionParts holds parsed version components.
type versionParts struct {
	major int
	minor int
	patch int
}

// parseVersion splits a version string like "5.7.3" into components.
func parseVersion(v string) versionParts {
	parts := versionParts{major: 5, minor: 7, patch: 3}
	if v == "" {
		return parts
	}

	segments := extractVersionSegments(v)

	if len(segments) > 0 {
		parts.major = parseIntSimple(segments[0])
	}
	if len(segments) > 1 {
		parts.minor = parseIntSimple(segments[1])
	}
	if len(segments) > 2 {
		parts.patch = parseIntSimple(segments[2])
	}

	return parts
}

// extractVersionSegments extracts dot-separated digit segments from a version string.
func extractVersionSegments(v string) []string {
	var segments []string
	var current string

	for i := 0; i < len(v); i++ {
		if v[i] >= '0' && v[i] <= '9' {
			current += string(v[i])
		} else if current != "" {
			segments = append(segments, current)
			current = ""
		}
	}
	if current != "" {
		segments = append(segments, current)
	}

	return segments
}

// parseIntSimple converts a digit string to int without strconv.
func parseIntSimple(s string) int {
	if s == "" {
		return 0
	}
	result := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			result = result*10 + int(s[i]-'0')
		}
	}
	return result
}

// FakeProject creates a .uproject file + Content/ directory and returns the .uproject path.
func FakeProject(t *testing.T, name string) string {
	t.Helper()

	root := t.TempDir()
	uprojectPath := filepath.Join(root, name+".uproject")

	// Create minimal .uproject JSON
	uprojectData := map[string]interface{}{
		"FileVersion":       3,
		"EngineAssociation": "5.7",
		"Category":          "Games",
		"Description":       "Test project",
		"Modules":           []interface{}{},
		"TargetPlatforms":   []interface{}{"Linux"},
	}

	uprojectJSON, err := json.Marshal(uprojectData)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(uprojectPath, uprojectJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create Content/ directory
	contentDir := filepath.Join(root, "Content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	return uprojectPath
}
