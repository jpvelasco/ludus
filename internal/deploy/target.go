package deploy

import (
	"context"
	"io"
)

// Capabilities describes what a deployment target supports and needs.
type Capabilities struct {
	NeedsContainerBuild bool
	NeedsContainerPush  bool
	// NeedsAWS reports whether the target talks to AWS. Callers use it to
	// skip AWS prerequisite validation (AWS CLI + credentials) for targets
	// that never do (e.g. binary export). Any target with NeedsImageURI
	// also needs AWS.
	NeedsAWS bool
	// NeedsImageURI reports whether the target deploys the pushed ECR
	// container image. Callers resolve the ECR image URI only for these
	// targets; file-based targets (binary, ec2, anywhere) deploy from the
	// packaged server directory and need no ECR configuration.
	NeedsImageURI   bool
	SupportsSession bool
	SupportsDeploy  bool
	SupportsDestroy bool
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

// OutputSetter is optionally implemented by targets whose deployers can
// route their status prints and subprocess output to a specific writer
// (an MCP tool's per-operation capture). Targets that don't implement it
// keep their default global-stream behavior (#765).
type OutputSetter interface {
	SetOutput(w io.Writer)
}
