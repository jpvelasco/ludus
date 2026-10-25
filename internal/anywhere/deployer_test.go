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

	executable := serverExecutablePath("/opt/builds/LinuxServer", "Lyra", "Linux", "LyraServer")
	config := d.GenerateWrapperConfig(
		"arn:aws:gamelift:us-east-1::fleet/fleet-123",
		"arn:aws:gamelift:us-east-1::location/custom-ludus-dev",
		executable,
		"192.168.1.100",
	)

	assertContainsAll(t, config, []string{
		"anywhere:",
		"provider: aws-profile",
		"profile: default",
		"fleet-arn: arn:aws:gamelift:us-east-1::fleet/fleet-123",
		"location-arn: arn:aws:gamelift:us-east-1::location/custom-ludus-dev",
		"ipv4: 192.168.1.100",
		executable,
		"gamePort: 7777",
		`arg: "L_Expanse"`,
		`val: "7777"`,
	})

	if strings.Contains(config, "{{.ContainerPort}}") {
		t.Error("config should not contain container template variable")
	}
}

func TestPackagedDirName(t *testing.T) {
	tests := []struct {
		name            string
		projectName     string
		packagedDirName string
		want            string
	}{
		{"explicit packaged dir wins", "Lyra", "LyraStarterGame6", "LyraStarterGame6"},
		{"fallback to project name", "Lyra", "", "Lyra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := DeployOptions{ProjectName: tt.projectName, PackagedDirName: tt.packagedDirName}
			if got := o.packagedDirName(); got != tt.want {
				t.Errorf("packagedDirName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateWrapperConfig_PackagedDirDiffersFromProject(t *testing.T) {
	// The wrapper exec path must use the packaged content dir (the .uproject
	// name), not ProjectName, when they differ.
	d := &Deployer{
		opts: DeployOptions{
			ServerBuildDir:  "/opt/builds/LinuxServer",
			ProjectName:     "Lyra",
			PackagedDirName: "LyraStarterGame6",
			ServerTarget:    "LyraServer",
			ServerMap:       "L_Expanse",
			ServerPort:      7777,
			AWSProfile:      "default",
		},
	}
	executable := serverExecutablePath("/opt/builds/LinuxServer", "LyraStarterGame6", "Linux", "LyraServer")
	config := d.GenerateWrapperConfig("fleet-arn", "loc-arn", executable, "10.0.0.1")

	if !strings.Contains(config, executable) {
		t.Errorf("wrapper config should reference packaged dir path %q\ngot:\n%s", executable, config)
	}
	if strings.Contains(config, "/Lyra/Binaries") {
		t.Errorf("wrapper config must not use ProjectName 'Lyra' for the content dir\ngot:\n%s", config)
	}
}

func TestHostServerPlatformDir(t *testing.T) {
	platformDir, exeSuffix := hostServerPlatformDir()

	// Verify the platform directory and executable suffix follow the host OS
	switch runtime.GOOS {
	case "windows":
		if platformDir != "Win64" || exeSuffix != ".exe" {
			t.Errorf("hostServerPlatformDir() = %q, %q, want Win64, .exe", platformDir, exeSuffix)
		}
	default:
		wantDir := "Linux"
		if runtime.GOARCH == "arm64" {
			wantDir = "LinuxArm64"
		}
		if platformDir != wantDir || exeSuffix != "" {
			t.Errorf("hostServerPlatformDir() = %q, %q, want %q, empty", platformDir, exeSuffix, wantDir)
		}
	}
}

func TestServerExecutablePath(t *testing.T) {
	got := serverExecutablePath("/opt/builds/LinuxServer", "Lyra", "Linux", "LyraServer")

	want := filepath.ToSlash(filepath.Join("/opt/builds/LinuxServer", "Lyra", "Binaries", "Linux", "LyraServer"))
	if got != want {
		t.Errorf("serverExecutablePath() = %q, want %q", got, want)
	}
	if strings.Contains(got, `\`) {
		t.Errorf("serverExecutablePath() = %q, want forward slashes only", got)
	}
}

// TestGenerateWrapperConfigPortablePaths pins the YAML-safety contract for the
// generated wrapper config: the executable path is embedded in a quoted
// scalar, and backslashes are escape characters there. Windows-style build
// dirs must be normalized to forward slashes so the wrapper can parse its own
// config on every platform.
func TestGenerateWrapperConfigPortablePaths(t *testing.T) {
	d := &Deployer{
		opts: DeployOptions{
			ServerBuildDir: `C:\builds\PackagedServer`,
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
			ServerMap:      "L_Expanse",
			ServerPort:     7777,
			AWSProfile:     "default",
		},
	}

	platformDir, _ := hostServerPlatformDir()
	executable := serverExecutablePath(`C:\builds\PackagedServer`, "Lyra", platformDir, "LyraServer")
	config := d.GenerateWrapperConfig("fleet-arn", "loc-arn", executable, "192.168.1.100")

	if strings.Contains(config, `\`) {
		t.Errorf("wrapper config contains a raw backslash; not YAML-safe on Windows:\n%s", config)
	}
	assertContainsAll(t, config, []string{
		"executable-file-path:",
		"C:/builds/PackagedServer/Lyra/Binaries/",
	})
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
