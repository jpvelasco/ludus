package anywhere

import (
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectLocalIP(t *testing.T) {
	ip, err := DetectLocalIP()
	if err != nil {
		t.Skipf("no non-loopback IPv4 available: %v", err)
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("DetectLocalIP returned invalid IP: %q", ip)
	}
	if parsed.To4() == nil {
		t.Fatalf("DetectLocalIP returned non-IPv4 address: %q", ip)
	}
	if parsed.IsLoopback() {
		t.Fatalf("DetectLocalIP returned loopback address: %q", ip)
	}
}

// assertContainsAll checks that got contains all substrings in want.
func assertContainsAll(t *testing.T, got string, want []string) {
	t.Helper()
	for _, s := range want {
		if !strings.Contains(got, s) {
			t.Errorf("output missing %q", s)
		}
	}
}

func TestGenerateWrapperConfig(t *testing.T) {
	d := &Deployer{
		opts: DeployOptions{
			ServerBuildDir: "/opt/builds/LinuxServer",
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
			ServerMap:      "L_Expanse",
			ServerPort:     7777,
			AWSProfile:     "default",
		},
	}

	config := d.GenerateWrapperConfig(
		"arn:aws:gamelift:us-east-1::fleet/fleet-123",
		"arn:aws:gamelift:us-east-1::location/custom-ludus-dev",
		"/usr/local/bin/wrapper",
		"192.168.1.100",
	)

	expectedBinary := serverBinaryPath("/opt/builds/LinuxServer", "Lyra", "LyraServer")

	assertContainsAll(t, config, []string{
		"anywhere:",
		"provider: aws-profile",
		"profile: default",
		"fleet-arn: arn:aws:gamelift:us-east-1::fleet/fleet-123",
		"location-arn: arn:aws:gamelift:us-east-1::location/custom-ludus-dev",
		"ipv4: 192.168.1.100",
		expectedBinary,
		"gamePort: 7777",
		`arg: "L_Expanse"`,
		`val: "7777"`,
	})

	if strings.Contains(config, "{{.ContainerPort}}") {
		t.Error("config should not contain container template variable")
	}
}

func TestServerBinaryPath(t *testing.T) {
	got := serverBinaryPath("/opt/builds/LinuxServer", "Lyra", "LyraServer")

	// Verify it uses the correct platform directory and suffix for the host OS
	switch runtime.GOOS {
	case "windows":
		want := filepath.Join("/opt/builds/LinuxServer", "Lyra", "Binaries", "Win64", "LyraServer.exe")
		if got != want {
			t.Errorf("serverBinaryPath() = %q, want %q", got, want)
		}
	default:
		if runtime.GOARCH == "arm64" {
			if !strings.Contains(got, "LinuxArm64") {
				t.Errorf("serverBinaryPath() on arm64 should contain LinuxArm64, got %q", got)
			}
		} else {
			if !strings.Contains(got, filepath.Join("Binaries", "Linux")) {
				t.Errorf("serverBinaryPath() on amd64 should contain Binaries/Linux, got %q", got)
			}
		}
		if strings.HasSuffix(got, ".exe") {
			t.Errorf("serverBinaryPath() on Linux should not have .exe suffix, got %q", got)
		}
	}
}

func TestLocationNamePrefix(t *testing.T) {
	tests := []struct {
		input   string
		wantPfx string
	}{
		{"custom-ludus-dev", "custom-"},
		{"ludus-dev", "custom-"},
		{"custom-", "custom-"},
	}

	for _, tt := range tests {
		loc := tt.input
		if !strings.HasPrefix(loc, "custom-") {
			loc = "custom-" + loc
		}
		if !strings.HasPrefix(loc, tt.wantPfx) {
			t.Errorf("location %q doesn't start with %q", loc, tt.wantPfx)
		}
	}
}

func TestIsProcessAlive(t *testing.T) {
	// PID 0 or negative should return false
	if IsProcessAlive(0) {
		t.Error("IsProcessAlive(0) should be false")
	}
	if IsProcessAlive(-1) {
		t.Error("IsProcessAlive(-1) should be false")
	}
}
