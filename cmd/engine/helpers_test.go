package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readWSL2State loads the WSL2 engine state file and returns the parsed map.
func readWSL2State(t *testing.T, tmpDir string) map[string]interface{} {
	t.Helper()

	stateFile := filepath.Join(tmpDir, ".ludus", "state.json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	return state
}

// verifyWSL2StateFile reads and verifies the WSL2 engine state file.
func verifyWSL2StateFile(t *testing.T, tmpDir, expectedEnginePath, expectedDDCPath string) {
	t.Helper()

	state := readWSL2State(t, tmpDir)

	wsl2State, ok := state["wsl2Engine"].(map[string]interface{})
	if !ok {
		t.Fatalf("wsl2Engine field missing or wrong type in state")
	}

	enginePath, ok := wsl2State["enginePath"].(string)
	if !ok || enginePath != expectedEnginePath {
		t.Errorf("enginePath = %q, want %q", enginePath, expectedEnginePath)
	}

	ddcPath, ok := wsl2State["ddcPath"].(string)
	if !ok || ddcPath != expectedDDCPath {
		t.Errorf("ddcPath = %q, want %q", ddcPath, expectedDDCPath)
	}
}
