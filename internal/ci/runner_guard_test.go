package ci

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

// The Install/Status/Uninstall flows are Linux-only; their full bodies are
// covered by runner_unix_test.go on the CI ubuntu leg. These tests cover the
// non-Linux guard returns so they stay covered on the windows/macos legs too.

func TestRunnerInstallerInstallNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("linux install path is covered in runner_unix_test.go")
	}
	ri := &RunnerInstaller{}
	err := ri.Install(context.Background())
	if err == nil || !strings.Contains(err.Error(), "runner install is only supported on Linux") {
		t.Fatalf("Install() on %s = %v, want Linux-only guard error", runtime.GOOS, err)
	}
}

func TestRunnerInstallerStatusNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("linux status path is covered in runner_unix_test.go")
	}
	ri := &RunnerInstaller{}
	_, err := ri.Status(context.Background())
	if err == nil || !strings.Contains(err.Error(), "runner status is only supported on Linux") {
		t.Fatalf("Status() on %s = %v, want Linux-only guard error", runtime.GOOS, err)
	}
}

func TestRunnerInstallerUninstallNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("linux uninstall path is covered in runner_unix_test.go")
	}
	ri := &RunnerInstaller{}
	err := ri.Uninstall(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "runner uninstall is only supported on Linux") {
		t.Fatalf("Uninstall() on %s = %v, want Linux-only guard error", runtime.GOOS, err)
	}
}
