package deploy

import (
	"testing"
	"time"

	"github.com/jpvelasco/ludus/internal/state"
)

func TestSaveSessionState(t *testing.T) {
	setupSessionTest(t)
	before := time.Now().UTC().Add(-time.Second)
	want := &SessionInfo{SessionID: "session-123", IPAddress: "192.0.2.10", Port: 7777}
	SaveSessionState(want)
	assertSavedSession(t, loadState(t).Session, want, before)
}

func assertSavedSession(t *testing.T, got *state.SessionState, want *SessionInfo, before time.Time) {
	t.Helper()
	if got == nil {
		t.Fatal("session state is nil")
	}
	assertSessionFields(t, got, want)
	assertCurrentTimestamp(t, got.CreatedAt, before)
}

func assertSessionFields(t *testing.T, got *state.SessionState, want *SessionInfo) {
	t.Helper()
	if got.SessionID != want.SessionID {
		t.Errorf("session ID = %q, want %q", got.SessionID, want.SessionID)
	}
	if got.IPAddress != want.IPAddress {
		t.Errorf("IP address = %q, want %q", got.IPAddress, want.IPAddress)
	}
	if got.Port != want.Port {
		t.Errorf("port = %d, want %d", got.Port, want.Port)
	}
	if got.Status != "ACTIVE" {
		t.Errorf("session status = %q, want ACTIVE", got.Status)
	}
}

func assertCurrentTimestamp(t *testing.T, value string, before time.Time) {
	t.Helper()
	createdAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse CreatedAt %q: %v", value, err)
	}
	if createdAt.Before(before) || createdAt.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("CreatedAt = %v, want current time", createdAt)
	}
}
func TestClearFleetState(t *testing.T) {
	setupSessionTest(t)
	if err := state.UpdateFleet(&state.FleetState{FleetID: "fleet-123"}); err != nil {
		t.Fatalf("seed fleet state: %v", err)
	}
	if err := state.UpdateSession(&state.SessionState{SessionID: "session-123"}); err != nil {
		t.Fatalf("seed session state: %v", err)
	}
	ClearFleetState()
	got := loadState(t)
	if got.Fleet != nil {
		t.Errorf("fleet state = %#v after clear, want nil", got.Fleet)
	}
	if got.Session != nil {
		t.Errorf("session state = %#v after clear, want nil", got.Session)
	}
}

func setupSessionTest(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	previous := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(previous) })
}

func loadState(t *testing.T) *state.State {
	t.Helper()
	got, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	return got
}
