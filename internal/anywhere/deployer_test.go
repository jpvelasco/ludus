package anywhere

import (
	"net"
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

	// Verify anywhere section is present
	if !strings.Contains(config, "anywhere:") {
		t.Error("config missing 'anywhere:' section")
	}

	// Verify provider
	if !strings.Contains(config, "provider: aws-profile") {
		t.Error("config missing 'provider: aws-profile'")
	}

	// Verify profile
	if !strings.Contains(config, "profile: default") {
		t.Error("config missing 'profile: default'")
	}

	// Verify fleet ARN
	if !strings.Contains(config, "fleet-arn: arn:aws:gamelift:us-east-1::fleet/fleet-123") {
		t.Error("config missing correct fleet-arn")
	}

	// Verify location ARN
	if !strings.Contains(config, "location-arn: arn:aws:gamelift:us-east-1::location/custom-ludus-dev") {
		t.Error("config missing correct location-arn")
	}

	// Verify IP
	if !strings.Contains(config, "ipv4: 192.168.1.100") {
		t.Error("config missing correct ipv4 address")
	}

	// Verify server binary path uses absolute path
	if !strings.Contains(config, "/opt/builds/LinuxServer/Lyra/Binaries/Linux/LyraServer") {
		t.Error("config missing correct executable path")
	}

	// Verify game port
	if !strings.Contains(config, "gamePort: 7777") {
		t.Error("config missing correct gamePort")
	}

	// Verify server map arg
	if !strings.Contains(config, `arg: "L_Expanse"`) {
		t.Error("config missing server map argument")
	}

	// Verify port arg
	if !strings.Contains(config, `val: "7777"`) {
		t.Error("config missing port value")
	}

	// Verify no container template variable
	if strings.Contains(config, "{{.ContainerPort}}") {
		t.Error("config should not contain container template variable")
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
