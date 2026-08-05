package wsl

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestDetectAvailable(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		Stdout: "* Ubuntu          Running         2",
	})

	info, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !info.Available {
		t.Fatal("Detect() Available = false, want true")
	}
	if len(info.Distros) != 1 || info.Distros[0].Name != "Ubuntu" {
		t.Errorf("Detect() Distros = %+v, want [Ubuntu]", info.Distros)
	}
}

func TestDetectCommandError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})

	info, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if info.Available {
		t.Fatal("Detect() Available = true, want false")
	}
}

func TestDetectNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	info, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if info.Available {
		t.Fatal("Detect() Available = true, want false")
	}
}

func TestCheckDepsAllPresent(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{})
	r := newTestRunner(t, false)

	if err := CheckDeps(context.Background(), r, "Ubuntu"); err != nil {
		t.Errorf("CheckDeps() error = %v, want nil", err)
	}
}

func TestCheckDepsMissing(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	r := newTestRunner(t, false)

	err := CheckDeps(context.Background(), r, "Ubuntu")
	if err == nil || !strings.Contains(err.Error(), "missing build dependencies") {
		t.Errorf("CheckDeps() error = %v, want 'missing build dependencies'", err)
	}
}

func TestCheckRuntimeDepsPresent(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{})
	r := newTestRunner(t, false)

	if err := CheckRuntimeDeps(context.Background(), r, "Ubuntu"); err != nil {
		t.Errorf("CheckRuntimeDeps() error = %v, want nil", err)
	}
}

func TestCheckRuntimeDepsMissing(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	r := newTestRunner(t, false)

	err := CheckRuntimeDeps(context.Background(), r, "Ubuntu")
	if err == nil || !strings.Contains(err.Error(), "runtime libraries missing") {
		t.Errorf("CheckRuntimeDeps() error = %v, want 'runtime libraries missing'", err)
	}
}

func TestCheckDiskSpaceSuccess(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})
	r := newTestRunner(t, false)

	got, err := CheckDiskSpace(context.Background(), r, "Ubuntu")
	if err != nil {
		t.Fatalf("CheckDiskSpace() error = %v", err)
	}
	if got != 250 {
		t.Errorf("CheckDiskSpace() = %v, want 250", got)
	}
}

func TestCheckDiskSpaceCommandError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	r := newTestRunner(t, false)

	if _, err := CheckDiskSpace(context.Background(), r, "Ubuntu"); err == nil {
		t.Error("CheckDiskSpace() error = nil, want command error")
	}
}

func TestCheckDiskSpaceParseError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "not-a-number"})
	r := newTestRunner(t, false)

	if _, err := CheckDiskSpace(context.Background(), r, "Ubuntu"); err == nil {
		t.Error("CheckDiskSpace() error = nil, want parse error")
	}
}

func TestCheckRsync(t *testing.T) {
	tests := []struct {
		name     string
		behavior testsupport.ToolBehavior
		want     bool
	}{
		{"available", testsupport.ToolBehavior{}, true},
		{"missing", testsupport.ToolBehavior{ExitCode: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "wsl.exe", tt.behavior)
			r := newTestRunner(t, false)

			if got := CheckRsync(context.Background(), r, "Ubuntu"); got != tt.want {
				t.Errorf("CheckRsync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstallDeps(t *testing.T) {
	tests := []struct {
		name     string
		install  func(context.Context, *runner.Runner, string) error
		exitCode int
		wantErr  bool
	}{
		{"deps success", InstallDeps, 0, false},
		{"deps failure", InstallDeps, 1, true},
		{"runtime success", InstallRuntimeDeps, 0, false},
		{"runtime failure", InstallRuntimeDeps, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: tt.exitCode})
			r := newTestRunner(t, false)

			err := tt.install(context.Background(), r, "Ubuntu")
			if (err != nil) != tt.wantErr {
				t.Errorf("install() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDistroNames(t *testing.T) {
	got := distroNames([]DistroInfo{{Name: "Ubuntu"}, {Name: "Debian"}})
	if got != "Ubuntu, Debian" {
		t.Errorf("distroNames() = %q, want %q", got, "Ubuntu, Debian")
	}
}
