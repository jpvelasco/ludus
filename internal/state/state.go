package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateDir = ".ludus"
const stateFile = "state.json"

// State holds persistent pipeline state across commands.
type State struct {
	Fleet   *FleetState   `json:"fleet,omitempty"`
	Session *SessionState `json:"session,omitempty"`
	Client  *ClientState  `json:"client,omitempty"`
	Deploy  *DeployState  `json:"deploy,omitempty"`
}

// FleetState tracks the deployed GameLift fleet.
type FleetState struct {
	FleetID   string `json:"fleetId"`
	StackName string `json:"stackName,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// SessionState tracks the active game session.
type SessionState struct {
	SessionID string `json:"sessionId"`
	IPAddress string `json:"ipAddress"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// ClientState tracks the most recent client build.
type ClientState struct {
	BinaryPath string `json:"binaryPath"`
	OutputDir  string `json:"outputDir"`
	Platform   string `json:"platform"`
	BuiltAt    string `json:"builtAt"`
}

// DeployState tracks the most recent deployment.
type DeployState struct {
	TargetName string `json:"targetName"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	DeployedAt string `json:"deployedAt"`
}

func statePath() string {
	return filepath.Join(stateDir, stateFile)
}

// Load reads .ludus/state.json, returning an empty State if the file is missing.
func Load() (*State, error) {
	data, err := os.ReadFile(statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}

	s := &State{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Save writes state to .ludus/state.json with indentation, creating the directory if needed.
func Save(s *State) error {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath(), data, 0644)
}

// UpdateFleet loads state, updates the fleet block, and saves.
func UpdateFleet(fleet *FleetState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Fleet = fleet
	return Save(s)
}

// UpdateSession loads state, updates the session block, and saves.
func UpdateSession(session *SessionState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Session = session
	return Save(s)
}

// UpdateClient loads state, updates the client block, and saves.
func UpdateClient(client *ClientState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Client = client
	return Save(s)
}

// ClearSession sets session to nil.
func ClearSession() error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Session = nil
	return Save(s)
}

// ClearFleet sets both fleet and session to nil.
func ClearFleet() error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Fleet = nil
	s.Session = nil
	return Save(s)
}

// UpdateDeploy loads state, updates the deploy block, and saves.
func UpdateDeploy(deploy *DeployState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Deploy = deploy
	return Save(s)
}
