package connect

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// stubTarget is a hermetic deploy.Target that implements deploy.SessionManager.
type stubTarget struct {
	name         string
	supportsSess bool
	describe     string
	describeErr  error
}

func (s *stubTarget) Name() string { return s.name }
func (s *stubTarget) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{SupportsSession: s.supportsSess}
}
func (s *stubTarget) Deploy(context.Context, deploy.DeployInput) (*deploy.DeployResult, error) {
	return nil, nil
}
func (s *stubTarget) Status(context.Context) (*deploy.DeployStatus, error) { return nil, nil }
func (s *stubTarget) Destroy(context.Context) error                        { return nil }
func (s *stubTarget) CreateSession(context.Context, int) (*deploy.SessionInfo, error) {
	return nil, nil
}
func (s *stubTarget) DescribeSession(context.Context, string) (string, error) {
	return s.describe, s.describeErr
}

// plainTarget implements deploy.Target but NOT deploy.SessionManager.
type plainTarget struct{ name string }

func (s *plainTarget) Name() string { return s.name }
func (s *plainTarget) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{SupportsSession: false}
}
func (s *plainTarget) Deploy(context.Context, deploy.DeployInput) (*deploy.DeployResult, error) {
	return nil, nil
}
func (s *plainTarget) Status(context.Context) (*deploy.DeployStatus, error) { return nil, nil }
func (s *plainTarget) Destroy(context.Context) error                        { return nil }

// swapTarget routes ResolveTarget to return the given target.
func swapTarget(t *testing.T, target deploy.Target, err error) {
	t.Helper()
	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return target, err
	})
}

// createClientBinary writes a fake client executable and returns its path.
func createClientBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "client.exe")
	if err := os.WriteFile(binary, []byte("bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary
}

// safeLaunchPlatform returns a client platform that makes launchClient return
// nil without actually exec-ing a process on the current host: on Windows the
// Linux path just prints a hint, on Unix the Win64 path prints copy
// instructions. This lets runConnect success flows be tested hermetically.
func safeLaunchPlatform() string {
	if runtime.GOOS == "windows" {
		return "Linux"
	}
	return "Win64"
}

func TestRunConnectFlow(t *testing.T) {
	binary := createClientBinary(t)

	tests := []connectFlowCase{
		{
			name:    "no session no client",
			wantErr: "no active game session",
		},
		{
			name:    "session but no client",
			session: &state.SessionState{IPAddress: "10.1.1.1", Port: 9000, SessionID: "s1"},
			wantErr: "no client build found",
		},
		{
			name:    "client binary missing on disk",
			client:  &state.ClientState{BinaryPath: filepath.Join(t.TempDir(), "nope"), Platform: "Linux"},
			session: &state.SessionState{IPAddress: "10.1.1.1", Port: 9000},
			wantErr: "client binary not found",
		},
		{
			name:    "active session with session-managing target",
			client:  &state.ClientState{BinaryPath: binary, Platform: safeLaunchPlatform(), OutputDir: t.TempDir()},
			session: &state.SessionState{IPAddress: "10.1.1.1", Port: 9000, SessionID: "s1"},
			target:  &stubTarget{name: "gamelift", supportsSess: true, describe: "ACTIVE"},
		},
		{
			name:    "terminated session errors",
			client:  &state.ClientState{BinaryPath: binary, Platform: "Linux", OutputDir: t.TempDir()},
			session: &state.SessionState{IPAddress: "10.1.1.1", Port: 9000, SessionID: "s1"},
			target:  &stubTarget{name: "gamelift", supportsSess: true, describe: "TERMINATED"},
			wantErr: "is TERMINATED",
		},
		{
			name:    "describe error",
			client:  &state.ClientState{BinaryPath: binary, Platform: "Linux", OutputDir: t.TempDir()},
			session: &state.SessionState{IPAddress: "10.1.1.1", Port: 9000, SessionID: "s1"},
			target:  &stubTarget{name: "gamelift", supportsSess: true, describeErr: context.DeadlineExceeded},
			wantErr: "game session check failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runConnectFlowCase(t, tt)
		})
	}
}

type connectFlowCase struct {
	name    string
	client  *state.ClientState
	session *state.SessionState
	target  deploy.Target
	wantErr string
}

func runConnectFlowCase(t *testing.T, tt connectFlowCase) {
	t.Helper()
	oldAddress := address
	address = ""
	t.Cleanup(func() {
		address = oldAddress
		state.SetProfile("")
	})
	t.Chdir(t.TempDir())

	if tt.session != nil {
		if err := state.UpdateSession(tt.session); err != nil {
			t.Fatal(err)
		}
	}
	if tt.client != nil {
		if err := state.UpdateClient(tt.client); err != nil {
			t.Fatal(err)
		}
	}
	globals.SetGlobals(t, &config.Config{Game: config.GameConfig{ProjectName: "Lyra"}})
	swapTarget(t, tt.target, nil)

	assertConnectResult(t, runConnect(Cmd, nil), tt.wantErr)
}

func assertConnectResult(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("runConnect() error = %v, want containing %q", err, wantErr)
		}
		return
	}
	if err != nil {
		t.Errorf("runConnect() unexpected error = %v", err)
	}
}

type connectAddressCase struct {
	name    string
	value   string
	client  *state.ClientState
	wantErr string
}

func TestRunConnectAddressOverride(t *testing.T) {
	binary := createClientBinary(t)

	tests := []connectAddressCase{
		{name: "invalid override", value: "bogus", wantErr: "invalid address format"},
		{name: "valid override no client", value: "127.0.0.1:9999", wantErr: "no client build found"},
		{
			name:   "valid override with client",
			value:  "127.0.0.1:9999",
			client: &state.ClientState{BinaryPath: binary, Platform: safeLaunchPlatform(), OutputDir: t.TempDir()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runConnectAddressCase(t, tt)
		})
	}
}

func runConnectAddressCase(t *testing.T, tt connectAddressCase) {
	t.Helper()
	oldAddress := address
	address = tt.value
	t.Cleanup(func() {
		address = oldAddress
		state.SetProfile("")
	})
	t.Chdir(t.TempDir())
	if tt.client != nil {
		if err := state.UpdateClient(tt.client); err != nil {
			t.Fatal(err)
		}
	}
	globals.SetGlobals(t, &config.Config{Game: config.GameConfig{ProjectName: "Lyra"}})

	assertConnectResult(t, runConnect(Cmd, nil), tt.wantErr)
}

func TestVerifyActiveSession(t *testing.T) {
	tests := []struct {
		name    string
		target  deploy.Target
		err     error
		wantErr string
	}{
		{name: "resolve error warns and continues", err: context.Canceled},
		{name: "non-session-manager target", target: &plainTarget{name: "binary"}},
		{name: "active", target: &stubTarget{name: "g", supportsSess: true, describe: "ACTIVE"}},
		{name: "terminated", target: &stubTarget{name: "g", supportsSess: true, describe: "TERMINATED"}, wantErr: "is TERMINATED"},
		{name: "describe error", target: &stubTarget{name: "g", supportsSess: true, describeErr: os.ErrPermission}, wantErr: "game session check failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, &config.Config{})
			swapTarget(t, tt.target, tt.err)
			err := verifyActiveSession(Cmd, &state.State{Session: &state.SessionState{SessionID: "s1"}})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("verifyActiveSession() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("verifyActiveSession() unexpected error = %v", err)
			}
		})
	}
}

func TestCheckSessionlessTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  deploy.Target
		err     error
		wantErr string
	}{
		{name: "resolve error returns nil", err: context.DeadlineExceeded},
		{name: "no session support errors", target: &plainTarget{name: "binary"}, wantErr: "does not support game sessions"},
		{name: "session support ok", target: &stubTarget{name: "gamelift", supportsSess: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, &config.Config{})
			swapTarget(t, tt.target, tt.err)
			err := checkSessionlessTarget(Cmd)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("checkSessionlessTarget() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("checkSessionlessTarget() unexpected error = %v", err)
			}
		})
	}
}

func TestVerifySessionRoutes(t *testing.T) {
	oldAddress := address
	address = ""
	t.Cleanup(func() {
		address = oldAddress
		state.SetProfile("")
	})
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})
	swapTarget(t, &stubTarget{name: "g", supportsSess: true, describe: "ACTIVE"}, nil)

	// non-nil session routes to verifyActiveSession.
	if err := verifySession(Cmd, &state.State{Session: &state.SessionState{SessionID: "s1"}}); err != nil {
		t.Errorf("verifySession() with session error = %v", err)
	}

	// nil session routes to checkSessionlessTarget.
	swapTarget(t, &plainTarget{name: "binary"}, nil)
	if err := verifySession(Cmd, &state.State{}); err == nil || !strings.Contains(err.Error(), "does not support game sessions") {
		t.Errorf("verifySession() nil-session error = %v, want sessionless-target error", err)
	}
}
