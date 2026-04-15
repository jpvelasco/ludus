package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

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
