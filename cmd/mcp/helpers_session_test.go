package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/deploy"
)

type sessionTestTarget struct {
	session *deploy.SessionInfo
	err     error
	calls   int
}

func (*sessionTestTarget) Name() string { return "test" }

func (*sessionTestTarget) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{SupportsSession: true}
}

func (*sessionTestTarget) Deploy(context.Context, deploy.DeployInput) (*deploy.DeployResult, error) {
	return nil, nil
}

func (*sessionTestTarget) Status(context.Context) (*deploy.DeployStatus, error) { return nil, nil }

func (*sessionTestTarget) Destroy(context.Context) error { return nil }

func (t *sessionTestTarget) CreateSession(context.Context, int) (*deploy.SessionInfo, error) {
	t.calls++
	return t.session, t.err
}

func (*sessionTestTarget) DescribeSession(context.Context, string) (string, error) { return "", nil }

type sessionTestReceiver struct {
	id   string
	ip   string
	port int
	err  string
}

func (r *sessionTestReceiver) setSession(id, ip string, port int) {
	r.id, r.ip, r.port = id, ip, port
}

func (r *sessionTestReceiver) setSessionError(msg string) {
	r.err = msg
}

func TestTryCreateSession(t *testing.T) {
	tests := []struct {
		name        string
		withSession bool
		target      deploy.Target
		wantCalls   int
		wantID      string
		wantErr     string
	}{
		{name: "disabled", target: &sessionTestTarget{}, wantCalls: 0},
		{name: "unsupported", withSession: true, target: targetOnly{Target: &sessionTestTarget{}}, wantCalls: 0, wantErr: "does not support game sessions"},
		{name: "error", withSession: true, target: &sessionTestTarget{err: errors.New("unavailable")}, wantCalls: 1, wantErr: "session creation failed"},
		{name: "nil session", withSession: true, target: &sessionTestTarget{}, wantCalls: 1, wantErr: "returned no session"},
		{name: "success", withSession: true, target: &sessionTestTarget{session: &deploy.SessionInfo{SessionID: "session-1", IPAddress: "192.0.2.10", Port: 7777}}, wantCalls: 1, wantID: "session-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTryCreateSession(t, tt.target, tt.withSession, tt.wantCalls, tt.wantID, tt.wantErr)
		})
	}
}

func assertTryCreateSession(t *testing.T, target deploy.Target, withSession bool, wantCalls int, wantID, wantErr string) {
	t.Helper()
	receiver := &sessionTestReceiver{}
	tryCreateSession(context.Background(), target, withSession, receiver)
	if got := sessionCreateCalls(target); got != wantCalls {
		t.Errorf("CreateSession calls = %d, want %d", got, wantCalls)
	}
	assertSessionReceiver(t, receiver, wantID, wantErr)
}

func assertSessionReceiver(t *testing.T, receiver *sessionTestReceiver, wantID, wantErr string) {
	t.Helper()
	if receiver.id != wantID {
		t.Errorf("session ID = %q, want %q", receiver.id, wantID)
	}
	assertSessionError(t, receiver.err, wantErr)
	if wantID != "" && (receiver.ip != "192.0.2.10" || receiver.port != 7777) {
		t.Errorf("session endpoint = %s:%d", receiver.ip, receiver.port)
	}
}

func assertSessionError(t *testing.T, got, want string) {
	t.Helper()
	if want != "" && !strings.Contains(got, want) {
		t.Errorf("session error = %q, want %q", got, want)
	}
	if want == "" && got != "" {
		t.Errorf("session error = %q, want empty", got)
	}
}

type targetOnly struct{ deploy.Target }

func (targetOnly) Name() string { return "binary" }

func sessionCreateCalls(target deploy.Target) int {
	if manager, ok := target.(*sessionTestTarget); ok {
		return manager.calls
	}
	return 0
}

func TestDeployResultsSetSession(t *testing.T) {
	fleet := &deployFleetResult{}
	stack := &deployStackResult{}
	anywhere := &deployAnywhereResult{}
	ec2 := &deployEC2Result{}

	receivers := []sessionReceiver{fleet, stack, anywhere, ec2}
	for _, receiver := range receivers {
		receiver.setSession("session-2", "198.51.100.5", 7788)
	}

	for _, receiver := range receivers {
		receiver.setSessionError("session creation failed: unavailable")
	}

	got := []sessionTestReceiver{
		{id: fleet.SessionID, ip: fleet.SessionIP, port: fleet.SessionPort, err: fleet.SessionError},
		{id: stack.SessionID, ip: stack.SessionIP, port: stack.SessionPort, err: stack.SessionError},
		{id: anywhere.SessionID, ip: anywhere.SessionIP, port: anywhere.SessionPort, err: anywhere.SessionError},
		{id: ec2.SessionID, ip: ec2.SessionIP, port: ec2.SessionPort, err: ec2.SessionError},
	}
	for i, result := range got {
		if result.id != "session-2" || result.ip != "198.51.100.5" || result.port != 7788 || result.err != "session creation failed: unavailable" {
			t.Errorf("result %d session = %+v", i, result)
		}
	}
}
