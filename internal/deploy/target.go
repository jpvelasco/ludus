package deploy

import "context"

// Capabilities describes what a deployment target supports.
type Capabilities struct {
	NeedsContainerBuild bool
	NeedsContainerPush  bool
	SupportsSession     bool
	SupportsDeploy      bool
	SupportsDestroy     bool
}

// DeployInput provides data needed to deploy to a target.
type DeployInput struct {
	ImageURI       string // container-based targets (GameLift)
	ServerBuildDir string // file-based targets (binary)
	ServerPort     int
}

// DeployResult holds the outcome of a deployment.
type DeployResult struct {
	TargetName string
	Status     string
	Detail     string // target-specific info (fleet ID, output path, etc.)
}

// DeployStatus holds the current status of a deployment target.
type DeployStatus struct {
	TargetName string
	Status     string // "active", "not_deployed", "error"
	Detail     string
}

// SessionInfo holds connection details for a game session.
type SessionInfo struct {
	SessionID string
	IPAddress string
	Port      int
}

// Target abstracts a deployment backend (GameLift, binary export, etc.).
type Target interface {
	Name() string
	Capabilities() Capabilities
	Deploy(ctx context.Context, input DeployInput) (*DeployResult, error)
	Status(ctx context.Context) (*DeployStatus, error)
	Destroy(ctx context.Context) error
}

// SessionManager is optionally implemented by targets that support game sessions.
type SessionManager interface {
	CreateSession(ctx context.Context, maxPlayers int) (*SessionInfo, error)
	DescribeSession(ctx context.Context, sessionID string) (string, error)
}
