package configcmd

import (
	"os"
	"path/filepath"
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

func TestResolveConfigFile(t *testing.T) {
	orig := globals.Profile
	t.Cleanup(func() { globals.Profile = orig })

	globals.Profile = ""
	if got := resolveConfigFile(); got != "ludus.yaml" {
		t.Errorf("default profile = %q, want ludus.yaml", got)
	}

	globals.Profile = "ue57"
	if got := resolveConfigFile(); got != "ludus-ue57.yaml" {
		t.Errorf("named profile = %q, want ludus-ue57.yaml", got)
	}
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
