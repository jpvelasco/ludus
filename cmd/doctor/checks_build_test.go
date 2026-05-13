package doctor

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
)

func TestClientBinaryIssue(t *testing.T) {
	tests := []struct {
		name string
		st   *state.State
		want string
	}{
		{name: "nil client", st: &state.State{}, want: ""},
		{name: "empty binary path", st: &state.State{Client: &state.ClientState{BinaryPath: ""}}, want: ""},
		{name: "binary exists", st: &state.State{Client: &state.ClientState{BinaryPath: "."}}, want: ""},
		{name: "binary missing", st: &state.State{Client: &state.ClientState{BinaryPath: "/nonexistent/path/binary.exe"}}, want: "client binary missing: /nonexistent/path/binary.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clientBinaryIssue(tt.st)
			if got != tt.want {
				t.Errorf("clientBinaryIssue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFleetStateIssue(t *testing.T) {
	tests := []struct {
		name string
		st   *state.State
		want string
	}{
		{name: "nil deploy", st: &state.State{}, want: ""},
		{name: "deploy not active", st: &state.State{Deploy: &state.DeployState{Status: "idle"}}, want: ""},
		{name: "active with fleet", st: &state.State{Deploy: &state.DeployState{Status: "active"}, Fleet: &state.FleetState{}}, want: ""},
		{name: "active with ec2fleet", st: &state.State{Deploy: &state.DeployState{Status: "active"}, EC2Fleet: &state.EC2FleetState{}}, want: ""},
		{name: "active with anywhere", st: &state.State{Deploy: &state.DeployState{Status: "active"}, Anywhere: &state.AnywhereState{}}, want: ""},
		{name: "active no fleet", st: &state.State{Deploy: &state.DeployState{Status: "active"}}, want: "deploy marked active but no fleet state found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fleetStateIssue(tt.st)
			if got != tt.want {
				t.Errorf("fleetStateIssue() = %q, want %q", got, tt.want)
			}
		})
	}
}
