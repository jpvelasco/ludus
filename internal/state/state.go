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
	Fleet       *FleetState       `json:"fleet,omitempty"`
	Session     *SessionState     `json:"session,omitempty"`
	Client      *ClientState      `json:"client,omitempty"`
	Deploy      *DeployState      `json:"deploy,omitempty"`
	EngineImage *EngineImageState `json:"engineImage,omitempty"`
	Anywhere    *AnywhereState    `json:"anywhere,omitempty"`
	EC2Fleet    *EC2FleetState    `json:"ec2Fleet,omitempty"`
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

// EngineImageState tracks the most recent engine Docker image build.
type EngineImageState struct {
	ImageTag string `json:"imageTag"`
	Version  string `json:"version,omitempty"`
	BuiltAt  string `json:"builtAt"`
}

// EC2FleetState tracks a deployed GameLift Managed EC2 fleet.
type EC2FleetState struct {
	FleetID   string `json:"fleetId"`
	BuildID   string `json:"buildId"`
	S3Bucket  string `json:"s3Bucket"`
	S3Key     string `json:"s3Key"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// AnywhereState tracks a running Anywhere server and fleet.
type AnywhereState struct {
	PID          int    `json:"pid"`
	ComputeName  string `json:"computeName"`
	FleetID      string `json:"fleetId"`
	FleetARN     string `json:"fleetArn"`
	LocationName string `json:"locationName"`
	LocationARN  string `json:"locationArn"`
	IPAddress    string `json:"ipAddress"`
	ServerPort   int    `json:"serverPort"`
	StartedAt    string `json:"startedAt"`
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

// UpdateEngineImage loads state, updates the engine image block, and saves.
func UpdateEngineImage(img *EngineImageState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.EngineImage = img
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

// UpdateAnywhere loads state, updates the anywhere block, and saves.
func UpdateAnywhere(anywhere *AnywhereState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Anywhere = anywhere
	return Save(s)
}

// ClearAnywhere sets the anywhere block to nil.
func ClearAnywhere() error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.Anywhere = nil
	return Save(s)
}

// UpdateEC2Fleet loads state, updates the EC2 fleet block, and saves.
func UpdateEC2Fleet(ec2Fleet *EC2FleetState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.EC2Fleet = ec2Fleet
	return Save(s)
}

// ClearEC2Fleet sets the EC2 fleet block to nil.
func ClearEC2Fleet() error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.EC2Fleet = nil
	return Save(s)
}
