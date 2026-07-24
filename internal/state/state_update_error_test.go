package state

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUpdateAndClearErrorOnCorruptedState verifies that every Update*/Clear*
// helper propagates the Load() error instead of silently overwriting a state
// file it could not parse.
func TestUpdateAndClearErrorOnCorruptedState(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{"UpdateFleet", func() error { return UpdateFleet(&FleetState{FleetID: "f-1"}) }},
		{"UpdateSession", func() error { return UpdateSession(&SessionState{SessionID: "s-1"}) }},
		{"UpdateClient", func() error { return UpdateClient(&ClientState{Platform: "Linux"}) }},
		{"ClearSession", ClearSession},
		{"ClearFleet", ClearFleet},
		{"UpdateEngineImage", func() error { return UpdateEngineImage(&EngineImageState{ImageTag: "t"}) }},
		{"UpdateDeploy", func() error { return UpdateDeploy(&DeployState{TargetName: "gamelift"}) }},
		{"UpdateAnywhere", func() error { return UpdateAnywhere(&AnywhereState{FleetID: "a-1"}) }},
		{"ClearAnywhere", ClearAnywhere},
		{"UpdateEC2Fleet", func() error { return UpdateEC2Fleet(&EC2FleetState{FleetID: "ec2-1"}) }},
		{"ClearEC2Fleet", ClearEC2Fleet},
		{"UpdateWSL2Engine", func() error { return UpdateWSL2Engine(&WSL2EngineState{EnginePath: "/e"}) }},
		{"ClearWSL2Engine", ClearWSL2Engine},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTest(t)
			writeCorruptedState(t)

			if err := tt.fn(); err == nil {
				t.Fatalf("%s: expected error when state file is corrupted", tt.name)
			}
		})
	}
}

func writeCorruptedState(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, stateFile), []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
}
