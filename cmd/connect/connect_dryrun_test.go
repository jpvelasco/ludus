package connect

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// captureConnectStdout runs fn while redirecting stdout to a pipe and returns
// the captured output plus fn's error.
func captureConnectStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	prev := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = prev })
	runErr := run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = prev
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

// launchingPlatform returns a client platform whose launchClient would start
// a real process on the current host: on Unix the Linux client replaces the
// process via syscall.Exec, on Windows the Win64 client is started. A missing
// dry-run guard therefore surfaces as a launch error.
func launchingPlatform() string {
	if runtime.GOOS == "windows" {
		return "Win64"
	}
	return "Linux"
}

// connectClientState returns a saved client state pointing at a real file, so
// resolveClientBinary passes and connect reaches the launch step.
func connectClientState(t *testing.T, platform string) *state.ClientState {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "client")
	if err := os.WriteFile(binary, []byte("bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	return &state.ClientState{BinaryPath: binary, Platform: platform, OutputDir: t.TempDir()}
}

// TestRunConnectDryRunPreviewsLaunch pins #723: under --dry-run, connect
// prints the resolved executable and arguments instead of launching the game
// client.
func TestRunConnectDryRunPreviewsLaunch(t *testing.T) {
	client := connectClientState(t, launchingPlatform())
	tests := []struct {
		name    string
		address string
		session *state.SessionState
		want    []string
	}{
		{
			name:    "explicit address",
			address: "127.0.0.1:7777",
			want:    []string{"Dry run: would launch client: " + client.BinaryPath, "Dry run: connecting to: 127.0.0.1:7777", "Dry run: would run: " + client.BinaryPath + " 127.0.0.1:7777 -game -log"},
		},
		{
			name:    "active session",
			session: &state.SessionState{SessionID: "sess-1", IPAddress: "10.1.1.1", Port: 9000},
			want:    []string{"Dry run: session: sess-1", "Dry run: connecting to: 10.1.1.1:9000"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runConnectPreviewCase(t, tt.address, tt.session, client, tt.want)
		})
	}
}

func runConnectPreviewCase(t *testing.T, addr string, session *state.SessionState, client *state.ClientState, want []string) {
	t.Helper()
	oldAddress := address
	address = addr
	t.Cleanup(func() {
		address = oldAddress
		state.SetProfile("")
	})
	t.Chdir(t.TempDir())
	state.SetProfile("")

	if session != nil {
		if err := state.UpdateSession(session); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.UpdateClient(client); err != nil {
		t.Fatal(err)
	}
	// A failing aws stub keeps the session path's readiness check
	// side-effect free; it never fails Validate.
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{ExitCode: 1})
	swapTarget(t, &stubTarget{name: "gamelift", supportsSess: true, describe: "ACTIVE"}, nil)
	globals.SetGlobals(t, &config.Config{Game: config.GameConfig{ProjectName: "Lyra"}}, globals.WithDryRun(true))

	output, err := captureConnectStdout(t, func() error { return runConnect(Cmd, nil) })
	if err != nil {
		t.Fatalf("runConnect(dry-run) error = %v, want nil preview\n%s", err, output)
	}
	for _, wantLine := range want {
		if !strings.Contains(output, wantLine) {
			t.Errorf("preview missing %q; output:\n%s", wantLine, output)
		}
	}
}

// TestRunConnectAWSReadinessOrdering pins #722: an explicit --address connects
// directly without any AWS readiness check, while the session path keeps it.
// A failing aws stub makes the check observable: it prints an AWS Credentials
// warning line whenever it runs.
func TestRunConnectAWSReadinessOrdering(t *testing.T) {
	client := connectClientState(t, safeLaunchPlatform())
	tests := []struct {
		name        string
		address     string
		session     *state.SessionState
		wantAWSLine bool
	}{
		{name: "explicit address skips AWS readiness", address: "127.0.0.1:7777", wantAWSLine: false},
		{
			name:        "session path runs AWS readiness",
			session:     &state.SessionState{SessionID: "sess-2", IPAddress: "10.1.1.1", Port: 9000},
			wantAWSLine: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runConnectAWSOrderingCase(t, tt.address, tt.session, client, tt.wantAWSLine)
		})
	}
}

func runConnectAWSOrderingCase(t *testing.T, addr string, session *state.SessionState, client *state.ClientState, wantAWSLine bool) {
	t.Helper()
	oldAddress := address
	address = addr
	t.Cleanup(func() {
		address = oldAddress
		state.SetProfile("")
	})
	t.Chdir(t.TempDir())
	state.SetProfile("")

	if session != nil {
		if err := state.UpdateSession(session); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.UpdateClient(client); err != nil {
		t.Fatal(err)
	}
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{ExitCode: 1})
	swapTarget(t, &stubTarget{name: "gamelift", supportsSess: true, describe: "ACTIVE"}, nil)
	globals.SetGlobals(t, &config.Config{Game: config.GameConfig{ProjectName: "Lyra"}})

	output, err := captureConnectStdout(t, func() error { return runConnect(Cmd, nil) })
	if err != nil {
		t.Fatalf("runConnect() error = %v, want nil\n%s", err, output)
	}
	gotAWSLine := strings.Contains(output, "AWS Credentials")
	if gotAWSLine != wantAWSLine {
		t.Errorf("AWS readiness line present = %v, want %v; output:\n%s", gotAWSLine, wantAWSLine, output)
	}
}

// TestPreviewLaunch covers the #723 preview itself: the executable, session,
// address, and launch arguments are printed without launching anything.
func TestPreviewLaunch(t *testing.T) {
	output, err := captureConnectStdout(t, func() error {
		return previewLaunch("/games/Lyra.sh", "10.2.3.4:7777", "sess-3")
	})
	if err != nil {
		t.Fatalf("previewLaunch() error = %v", err)
	}
	for _, want := range []string{
		"Dry run: would launch client: /games/Lyra.sh",
		"Dry run: session: sess-3",
		"Dry run: connecting to: 10.2.3.4:7777",
		"Dry run: would run: /games/Lyra.sh 10.2.3.4:7777 -game -log",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("previewLaunch() output missing %q; got:\n%s", want, output)
		}
	}
}

// TestPreviewLaunchWithoutSession covers the sessionless preview: no session
// line is printed.
func TestPreviewLaunchWithoutSession(t *testing.T) {
	output, err := captureConnectStdout(t, func() error {
		return previewLaunch("/games/Lyra.sh", "127.0.0.1:7777", "")
	})
	if err != nil {
		t.Fatalf("previewLaunch() error = %v", err)
	}
	if strings.Contains(output, "Dry run: session:") {
		t.Errorf("previewLaunch() printed a session line without a session:\n%s", output)
	}
}

// TestActiveSessionID covers the session-ID extraction used by the preview.
func TestActiveSessionID(t *testing.T) {
	tests := []struct {
		name string
		s    *state.State
		want string
	}{
		{name: "nil state", s: nil, want: ""},
		{name: "no session", s: &state.State{}, want: ""},
		{name: "active session", s: &state.State{Session: &state.SessionState{SessionID: "s1"}}, want: "s1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activeSessionID(tt.s); got != tt.want {
				t.Errorf("activeSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
}
