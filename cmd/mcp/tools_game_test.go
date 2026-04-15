package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGameBuildInputWSL2Fields(t *testing.T) {
	input := gameBuildInput{
		Backend:   "wsl2",
		WSLDistro: "Debian",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded gameBuildInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Backend != "wsl2" {
		t.Errorf("Backend = %q, want %q", decoded.Backend, "wsl2")
	}
	if decoded.WSLDistro != "Debian" {
		t.Errorf("WSLDistro = %q, want %q", decoded.WSLDistro, "Debian")
	}
}

func TestGameBuildInputWSL2FieldsOmitEmpty(t *testing.T) {
	input := gameBuildInput{Backend: "native"}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	s := string(data)
	if strings.Contains(s, "wsl_distro") {
		t.Errorf("wsl_distro should be omitted when empty, got: %s", s)
	}
}
