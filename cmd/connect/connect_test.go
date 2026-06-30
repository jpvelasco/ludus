package connect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
)

func TestResolveClientBinary(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "client")
	if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		client  *state.ClientState
		want    string
		wantErr string
	}{
		{name: "missing state", wantErr: "no client build found"},
		{name: "empty path", client: &state.ClientState{}, wantErr: "no client build found"},
		{name: "missing binary", client: &state.ClientState{BinaryPath: binary + "-missing"}, wantErr: "client binary not found"},
		{name: "existing binary", client: &state.ClientState{BinaryPath: binary}, want: binary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveClientBinary(&state.State{Client: tt.client})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("resolveClientBinary() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("resolveClientBinary() = (%q, %v), want (%q, nil)", got, err, tt.want)
			}
		})
	}
}

func TestResolveAddressOverride(t *testing.T) {
	oldAddress := address
	t.Cleanup(func() { address = oldAddress })
	tests := []struct {
		name     string
		value    string
		wantIP   string
		wantPort int
		wantErr  string
	}{
		{name: "valid", value: "127.0.0.1:7777", wantIP: "127.0.0.1", wantPort: 7777},
		{name: "missing port", value: "127.0.0.1", wantErr: "expected ip:port"},
		{name: "invalid port", value: "127.0.0.1:nope", wantErr: "invalid port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			address = tt.value
			ip, port, err := resolveAddress()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("resolveAddress() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || ip != tt.wantIP || port != tt.wantPort {
				t.Errorf("resolveAddress() = (%q, %d, %v), want (%q, %d, nil)", ip, port, err, tt.wantIP, tt.wantPort)
			}
		})
	}
}

func TestResolveAddressFromState(t *testing.T) {
	oldAddress := address
	address = ""
	t.Cleanup(func() {
		address = oldAddress
		state.SetProfile("")
	})
	t.Chdir(t.TempDir())

	if _, _, err := resolveAddress(); err == nil || !strings.Contains(err.Error(), "no active game session") {
		t.Fatalf("resolveAddress() missing session error = %v", err)
	}
	want := &state.SessionState{IPAddress: "10.0.0.5", Port: 9000}
	if err := state.UpdateSession(want); err != nil {
		t.Fatal(err)
	}
	ip, port, err := resolveAddress()
	if err != nil || ip != want.IPAddress || port != want.Port {
		t.Errorf("resolveAddress() = (%q, %d, %v), want (%q, %d, nil)", ip, port, err, want.IPAddress, want.Port)
	}
}

func TestVerifySessionAddressOverride(t *testing.T) {
	oldAddress := address
	address = "localhost:7777"
	t.Cleanup(func() { address = oldAddress })
	if err := verifySession(Cmd, &state.State{}); err != nil {
		t.Errorf("verifySession() with override error = %v", err)
	}
}
