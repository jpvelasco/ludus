package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	WSL2Engine  *WSL2EngineState  `json:"wsl2Engine,omitempty"`
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

// WSL2EngineState tracks a WSL2-built engine.
// State is always fully populated after a successful build:
//
//	Default mode:  IsNative=false, EnginePath="/mnt/f/...", DDCPath="/mnt/f/.ludus/ddc/"
//	Native mode:   IsNative=true,  EnginePath="~/ludus/engine/5.7/", DDCPath="~/ludus/ddc/"
type WSL2EngineState struct {
	EnginePath string `json:"enginePath"`
	IsNative   bool   `json:"isNative"`
	DDCPath    string `json:"ddcPath"`
	SyncTime   string `json:"syncTime,omitempty"`
	BuiltAt    string `json:"builtAt"`
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

// activeProfile holds the current profile name. Empty string means the default
// profile (.ludus/state.json). Set via SetProfile().
var activeProfile string

// SetProfile sets the active state profile. Empty string means the default profile.
func SetProfile(name string) {
	activeProfile = name
}

// ActiveProfile returns the current profile name ("" for default).
func ActiveProfile() string {
	return activeProfile
}

func statePath() string {
	return statePathForProfile(activeProfile)
}

func statePathForProfile(profile string) string {
	if profile == "" {
		return filepath.Join(stateDir, stateFile)
	}
	return filepath.Join(stateDir, "profiles", profile+".json")
}

// Load reads the state file for the active profile, returning an empty State if missing.
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

// Save writes state to the active profile's file with indentation, creating directories as needed.
func Save(s *State) error {
	p := statePath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p, data, 0644)
}

// ListProfiles returns the names of all state profiles (excluding the default).
func ListProfiles() ([]string, error) {
	dir := filepath.Join(stateDir, "profiles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var profiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			profiles = append(profiles, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(profiles)
	return profiles, nil
}

// DeleteProfile removes a named profile's state file. Returns an error if the
// profile doesn't exist.
func DeleteProfile(name string) error {
	if name == "" {
		return fmt.Errorf("cannot delete the default profile")
	}
	p := statePathForProfile(name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return fmt.Errorf("profile %q does not exist", name)
	}
	return os.Remove(p)
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

// UpdateWSL2Engine loads state, updates the WSL2 engine block, and saves.
func UpdateWSL2Engine(ws *WSL2EngineState) error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.WSL2Engine = ws
	return Save(s)
}

// ClearWSL2Engine sets the WSL2 engine block to nil.
func ClearWSL2Engine() error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.WSL2Engine = nil
	return Save(s)
}
