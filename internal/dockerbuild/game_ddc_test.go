package dockerbuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestDDCArgs(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name       string
		ddcMode    string
		ddcPath    string
		wantErr    string
		wantArgs   []string
		wantNoArgs bool
	}{
		{
			name:    "local mode with empty path errors",
			ddcMode: "local",
			ddcPath: "",
			wantErr: "no path configured",
		},
		{
			name:    "unsupported mode errors",
			ddcMode: "s3",
			ddcPath: "/some/path",
			wantErr: "unsupported DDC mode",
		},
		{
			name:       "none mode returns no args",
			ddcMode:    "none",
			wantNoArgs: true,
		},
		{
			name:       "empty mode returns no args",
			ddcMode:    "",
			wantNoArgs: true,
		},
		{
			name:    "local mode returns volume and env args",
			ddcMode: "local",
			ddcPath: filepath.Join(t.TempDir(), "ddc"),
			wantArgs: []string{
				"-v", // volume mount present
				"/ddc",
				"-e",
				"UE-LocalDataCachePath=/ddc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(DockerGameOptions{DDCMode: tt.ddcMode, DDCPath: tt.ddcPath}, r)
			checkDDCArgs(t, b, tt.wantErr, tt.wantNoArgs, tt.wantArgs)
		})
	}
}

func TestDDCArgs_LocalCreatesDirectory(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{
		DDCMode: "local",
		DDCPath: ddcDir,
	}, r)

	_, err := b.ddcArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory should have been created: %v", err)
	}
}

func TestDDCArgs_LocalVolumeFormat(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{
		DDCMode: "local",
		DDCPath: ddcDir,
	}, r)

	args, err := b.ddcArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect exactly 4 args: -v <host>:/ddc -e UE-LocalDataCachePath=/ddc
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}
	if args[0] != "-v" {
		t.Errorf("args[0] = %q, want -v", args[0])
	}
	if !strings.HasSuffix(args[1], ":/ddc") {
		t.Errorf("args[1] = %q, should end with :/ddc", args[1])
	}
	if args[2] != "-e" {
		t.Errorf("args[2] = %q, want -e", args[2])
	}
	if args[3] != "UE-LocalDataCachePath=/ddc" {
		t.Errorf("args[3] = %q, want UE-LocalDataCachePath=/ddc", args[3])
	}
}

func TestDDCNotInBuildScript(t *testing.T) {
	tests := []struct {
		name string
		opts DockerGameOptions
	}{
		{
			name: "local mode does not embed DDC args in script",
			opts: DockerGameOptions{DDCMode: "local", DDCPath: "/tmp/ddc"},
		},
		{
			name: "none mode has no DDC args in script",
			opts: DockerGameOptions{DDCMode: "none"},
		},
		{
			name: "empty mode has no DDC args in script",
			opts: DockerGameOptions{},
		},
	}

	notExpected := []string{"-ini:Engine:[DerivedDataBackendGraph]", "UE-LocalDataCachePath"}

	r := runner.NewRunner(false, false)
	for _, tt := range tests {
		t.Run(tt.name+" (server)", func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.generateBuildScript(true)
			for _, notWant := range notExpected {
				if strings.Contains(got, notWant) {
					t.Errorf("server script should NOT contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
		t.Run(tt.name+" (client)", func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.generateBuildScript(false)
			for _, notWant := range notExpected {
				if strings.Contains(got, notWant) {
					t.Errorf("client script should NOT contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
	}
}
