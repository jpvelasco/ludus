package configcmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
)

func TestParseValue(t *testing.T) {
	tests := []struct {
		in   string
		want any
	}{
		{"true", true},
		{"TRUE", true},
		{"false", false},
		{"False", false},
		{"4", 4},
		{"0", 0},
		{"5.7", 5.7},
		{"-3", -3},
		{"hello", "hello"},
		{"c6g.large", "c6g.large"}, // not a float despite the dot
		{"5.7.3", "5.7.3"},         // version string, not numeric
		{"", ""},
	}
	for _, tt := range tests {
		if got := parseValue(tt.in); got != tt.want {
			t.Errorf("parseValue(%q) = %v (%T), want %v (%T)", tt.in, got, got, tt.want, tt.want)
		}
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{"plain", "plain"},
		{4, "4"},
		{true, "true"},
		{5.7, "5.7"},
	}
	for _, tt := range tests {
		if got := formatValue(tt.in); got != tt.want {
			t.Errorf("formatValue(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRunGetHonorsExplicitConfigFile reproduces #702: `ludus --config
// custom.yaml config get` must read the selected file even when the default
// ludus.yaml does not exist.
func TestRunGetHonorsExplicitConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("custom.yaml", []byte("engine:\n  version: \"5.7.3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globals.SetGlobals(t, nil, globals.WithConfigFile("custom.yaml"))

	output, err := captureConfigStdout(t, func() error { return runGet(nil, []string{"engine.version"}) })
	if err != nil {
		t.Fatalf("runGet() error = %v, want the value read from custom.yaml", err)
	}
	if !strings.Contains(output, "5.7.3") {
		t.Errorf("output = %q, want the value from custom.yaml", output)
	}
}

// TestRunSetHonorsExplicitConfigFile reproduces #702: `ludus --config
// custom.yaml config set` writes the selected file (preserving its other
// fields) and never creates the default ludus.yaml.
func TestRunSetHonorsExplicitConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seed := "engine:\n  version: \"5.7.3\"\naws:\n  region: eu-west-1\n"
	if err := os.WriteFile("custom.yaml", []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	globals.SetGlobals(t, nil, globals.WithConfigFile("custom.yaml"))

	if err := runSet(nil, []string{"engine.version", "5.8.0"}); err != nil {
		t.Fatalf("runSet() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ludus.yaml")); !os.IsNotExist(err) {
		t.Error("runSet() must not create ludus.yaml when --config selects another file")
	}

	data, err := os.ReadFile("custom.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"version: 5.8.0", "region: eu-west-1"} {
		if !strings.Contains(content, want) {
			t.Errorf("custom.yaml missing %q:\n%s", want, content)
		}
	}
}

func captureConfigStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	runErr := run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	t.Cleanup(func() { os.Stdout = previous })
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

func TestRunSetThenGet_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	orig := globals.Profile
	t.Cleanup(func() { globals.Profile = orig })
	globals.Profile = ""

	// set creates ludus.yaml and writes a typed value
	if err := runSet(nil, []string{"engine.version", "5.7.3"}); err != nil {
		t.Fatalf("runSet error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ludus.yaml")); err != nil {
		t.Fatalf("expected ludus.yaml created: %v", err)
	}

	// numeric value round-trips as int
	if err := runSet(nil, []string{"engine.maxJobs", "4"}); err != nil {
		t.Fatalf("runSet maxJobs error: %v", err)
	}

	// get reads back a set key without error
	if err := runGet(nil, []string{"engine.version"}); err != nil {
		t.Errorf("runGet existing key error: %v", err)
	}

	// get a missing key errors
	if err := runGet(nil, []string{"engine.doesNotExist"}); err == nil {
		t.Error("expected error for missing key")
	}
}

func TestRunGet_NoConfigFile(t *testing.T) {
	t.Chdir(t.TempDir())
	orig := globals.Profile
	t.Cleanup(func() { globals.Profile = orig })
	globals.Profile = ""

	if err := runGet(nil, []string{"engine.version"}); err == nil {
		t.Error("expected error when ludus.yaml absent")
	}
}

func TestRunSet_UnreadableConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	orig := globals.Profile
	t.Cleanup(func() { globals.Profile = orig })
	globals.Profile = ""

	// A malformed YAML file makes ReadInConfig fail with a non-not-found error.
	if err := os.WriteFile("ludus.yaml", []byte(": not: valid {yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runSet(nil, []string{"engine.version", "5.7.3"}); err == nil {
		t.Error("expected error reading malformed config")
	}
}

func TestRunGet_UnreadableConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	orig := globals.Profile
	t.Cleanup(func() { globals.Profile = orig })
	globals.Profile = ""

	if err := os.WriteFile("ludus.yaml", []byte("[unclosed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runGet(nil, []string{"engine.version"}); err == nil {
		t.Error("expected error reading malformed config")
	}
}
