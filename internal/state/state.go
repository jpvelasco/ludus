package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// ValidateProfileName checks that a --profile value maps to exactly one file
// directly beneath .ludus/profiles. The name is used as a path component, so
// separators, parent-directory segments, and absolute-path markers are
// rejected before any state read, write, or delete derives a path from it.
// Empty is valid: it selects the default profile.
func ValidateProfileName(name string) error {
	if name == "" {
		return nil
	}
	for i, r := range name {
		alnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		extra := r == '-' || r == '_' || r == '.'
		if !alnum && !extra || (i == 0 && !alnum) {
			return fmt.Errorf("invalid profile name %q: use letters, digits, '-', '_', or '.' with a leading letter or digit", name)
		}
	}
	return nil
}

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

// stateMu serializes state-file access within the process: read-modify-write
// helpers hold it across the whole sequence, and Load/Save take it for their
// single operation.
var stateMu sync.Mutex

// Load reads the state file for the active profile, returning an empty State if missing.
func Load() (*State, error) {
	stateMu.Lock()
	defer stateMu.Unlock()
	return loadUnlocked()
}

func loadUnlocked() (*State, error) {
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
// The write is atomic (temp file + rename), so a crash or concurrent reader
// never observes a truncated document.
func Save(s *State) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	return saveUnlocked(s)
}

func saveUnlocked(s *State) error {
	p := statePath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", p, err)
	}
	return nil
}

// mutate loads the active profile's state under the package lock, applies fn,
// and saves atomically — closing the lost-update window between Load and Save.
func mutate(fn func(s *State)) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	s, err := loadUnlocked()
	if err != nil {
		return err
	}
	fn(s)
	return saveUnlocked(s)
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
// profile doesn't exist or its name is not a safe single path component.
func DeleteProfile(name string) error {
	if name == "" {
		return fmt.Errorf("cannot delete the default profile")
	}
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	p := statePathForProfile(name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return fmt.Errorf("profile %q does not exist", name)
	}
	return os.Remove(p)
}

// UpdateFleet updates the fleet block atomically.
func UpdateFleet(fleet *FleetState) error {
	return mutate(func(s *State) { s.Fleet = fleet })
}

// UpdateSession updates the session block atomically.
func UpdateSession(session *SessionState) error {
	return mutate(func(s *State) { s.Session = session })
}

// UpdateClient updates the client block atomically.
func UpdateClient(client *ClientState) error {
	return mutate(func(s *State) { s.Client = client })
}

// ClearSession sets session to nil.
func ClearSession() error {
	return mutate(func(s *State) { s.Session = nil })
}

// ClearFleet sets both fleet and session to nil.
func ClearFleet() error {
	return mutate(func(s *State) { s.Fleet = nil; s.Session = nil })
}

// UpdateEngineImage updates the engine image block atomically.
func UpdateEngineImage(img *EngineImageState) error {
	return mutate(func(s *State) { s.EngineImage = img })
}

// UpdateDeploy updates the deploy block atomically.
func UpdateDeploy(deploy *DeployState) error {
	return mutate(func(s *State) { s.Deploy = deploy })
}

// UpdateAnywhere updates the anywhere block atomically.
func UpdateAnywhere(anywhere *AnywhereState) error {
	return mutate(func(s *State) { s.Anywhere = anywhere })
}

// ClearAnywhere sets the anywhere block to nil.
func ClearAnywhere() error {
	return mutate(func(s *State) { s.Anywhere = nil })
}

// UpdateEC2Fleet updates the EC2 fleet block atomically.
func UpdateEC2Fleet(ec2Fleet *EC2FleetState) error {
	return mutate(func(s *State) { s.EC2Fleet = ec2Fleet })
}

// ClearEC2Fleet sets the EC2 fleet block to nil.
func ClearEC2Fleet() error {
	return mutate(func(s *State) { s.EC2Fleet = nil })
}

// UpdateWSL2Engine updates the WSL2 engine block atomically.
func UpdateWSL2Engine(ws *WSL2EngineState) error {
	return mutate(func(s *State) { s.WSL2Engine = ws })
}

// ClearWSL2Engine sets the WSL2 engine block to nil.
func ClearWSL2Engine() error {
	return mutate(func(s *State) { s.WSL2Engine = nil })
}
