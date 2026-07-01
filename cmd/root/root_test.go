package root

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/spf13/cobra"
)

func TestRootCommandRegistration(t *testing.T) {
	names := make(map[string]bool)
	for _, command := range rootCmd.Commands() {
		names[command.Name()] = true
	}
	for _, want := range []string{
		"buildgraph", "ci", "config", "connect", "container", "ddc", "deploy",
		"doctor", "engine", "game", "init", "logs", "mcp", "resources", "run",
		"setup", "status",
	} {
		if !names[want] {
			t.Errorf("root command missing %q", want)
		}
	}
}

func TestPersistentPreRunRejectsInvalidDDCMode(t *testing.T) {
	configPath := writeRootConfig(t, "deploy:\n  target: binary\n")
	setRootGlobals(t)
	cfgFile = configPath
	globals.DDCMode = "invalid"

	err := rootCmd.PersistentPreRunE(namedRootCommand("mcp"), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid DDC mode") {
		t.Fatalf("PersistentPreRunE() error = %v, want invalid DDC mode", err)
	}
}

func TestPersistentPreRunUsesProfileConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	writeFile(t, filepath.Join(tempDir, "ludus-demo.yaml"), "deploy:\n  target: binary\n")
	setRootGlobals(t)
	globals.Profile = "demo"
	globals.Verbose = true

	stderr, err := captureRootStderr(t, func() error {
		return rootCmd.PersistentPreRunE(namedRootCommand("mcp"), nil)
	})
	if err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}
	if !strings.Contains(stderr, "Using profile config: ludus-demo.yaml") {
		t.Errorf("stderr = %q, want profile config notice", stderr)
	}
	if globals.Cfg.Deploy.Target != "binary" {
		t.Errorf("deploy target = %q, want binary", globals.Cfg.Deploy.Target)
	}
}

func TestPersistentPreRunDetectsEngineVersion(t *testing.T) {
	tempDir := t.TempDir()
	buildDir := filepath.Join(tempDir, "Engine", "Build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(buildDir, "Build.version"),
		`{"MajorVersion":5,"MinorVersion":7,"PatchVersion":3}`)
	configPath := writeRootConfig(t, "engine:\n  sourcePath: "+filepath.ToSlash(tempDir)+"\ndeploy:\n  target: binary\n")
	setRootGlobals(t)
	cfgFile = configPath
	globals.Verbose = true

	stderr, err := captureRootStderr(t, func() error {
		return rootCmd.PersistentPreRunE(namedRootCommand("mcp"), nil)
	})
	if err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}
	if globals.Cfg.Engine.Version != "5.7.3" {
		t.Errorf("engine version = %q, want 5.7.3", globals.Cfg.Engine.Version)
	}
	if !strings.Contains(stderr, "Auto-detected engine version: 5.7.3") {
		t.Errorf("stderr = %q, want auto-detection notice", stderr)
	}
}

func namedRootCommand(name string) *cobra.Command {
	command := &cobra.Command{Use: name}
	command.SetContext(context.Background())
	return command
}

func setRootGlobals(t *testing.T) {
	t.Helper()
	previousCfgFile := cfgFile
	previousCfg := globals.Cfg
	previousVerbose := globals.Verbose
	previousProfile := globals.Profile
	previousDDCMode := globals.DDCMode
	cfgFile = ""
	globals.Cfg = &config.Config{}
	globals.Verbose = false
	globals.Profile = ""
	globals.DDCMode = ""
	t.Cleanup(func() {
		cfgFile = previousCfgFile
		globals.Cfg = previousCfg
		globals.Verbose = previousVerbose
		globals.Profile = previousProfile
		globals.DDCMode = previousDDCMode
		state.SetProfile(previousProfile)
	})
}

func writeRootConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ludus.yaml")
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func captureRootStderr(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = writer
	runErr := run()
	os.Stderr = previous
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}
