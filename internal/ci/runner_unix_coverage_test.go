//go:build linux

package ci

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestRunnerInstallerStatusNotRunning(t *testing.T) {
	// svc.sh present but its status fails, and pgrep finds no listener.
	writeArgFailingTool(t, "sudo", 2, "status", "")
	testsupport.FakeTool(t, "pgrep", testsupport.ToolBehavior{ExitCode: 1})
	installer := &RunnerInstaller{Runner: realRunner(), InstallDir: t.TempDir()}
	writeRunnerMarkers(t, installer.InstallDir, true, true)

	got, err := installer.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if got != "installed, not running" {
		t.Errorf("Status() = %q, want %q", got, "installed, not running")
	}
}

func TestRunnerInstallerStatusProcessFallback(t *testing.T) {
	// svc.sh present but status fails; pgrep succeeds -> running (process).
	writeArgFailingTool(t, "sudo", 2, "status", "")
	testsupport.FakeTool(t, "pgrep", testsupport.ToolBehavior{})
	installer := &RunnerInstaller{Runner: realRunner(), InstallDir: t.TempDir()}
	writeRunnerMarkers(t, installer.InstallDir, true, true)

	got, err := installer.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if got != "running (process)" {
		t.Errorf("Status() = %q, want %q", got, "running (process)")
	}
}

func TestRunnerInstallerUninstallRemovalTokenError(t *testing.T) {
	testsupport.FakeTool(t, "gh", testsupport.ToolBehavior{ExitCode: 1})
	installer := &RunnerInstaller{Runner: realRunner(), InstallDir: t.TempDir()}
	writeRunnerMarkers(t, installer.InstallDir, true, true)

	err := installer.Uninstall(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "getting removal token") {
		t.Fatalf("Uninstall() error = %v, want removal-token error", err)
	}
}

func failingRunnerMarker(t *testing.T, dir, name, failArg string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"[ \"$1\" = \"" + failArg + "\" ] && exit 1\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerInstallerUninstallRemoveConfigError(t *testing.T) {
	testsupport.FakeTool(t, "gh", testsupport.ToolBehavior{})
	installer := &RunnerInstaller{Runner: realRunner(), InstallDir: t.TempDir()}
	// config.sh lives in the install dir and fails on "remove"; svc.sh succeeds.
	writeRunnerMarkers(t, installer.InstallDir, true, true)
	failingRunnerMarker(t, installer.InstallDir, "config.sh", "remove")

	err := installer.Uninstall(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "removing runner") {
		t.Fatalf("Uninstall() error = %v, want remove-config error", err)
	}
}

func TestRunnerInstallerUninstallNoServiceScript(t *testing.T) {
	testsupport.FakeTool(t, "gh", testsupport.ToolBehavior{})
	installer := &RunnerInstaller{Runner: realRunner(), InstallDir: t.TempDir()}
	// config.sh only; svc.sh absent so the service-stop block is skipped.
	os.MkdirAll(installer.InstallDir, 0o755)
	failingRunnerMarker(t, installer.InstallDir, "config.sh", "remove")

	err := installer.Uninstall(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "removing runner") {
		t.Fatalf("Uninstall() error = %v, want remove-config error", err)
	}
}

func TestRunnerInstallerUninstallKeepDir(t *testing.T) {
	installer := &RunnerInstaller{Runner: realRunner(), InstallDir: t.TempDir()}
	writeRunnerMarkers(t, installer.InstallDir, true, true)
	// gh and config.sh succeed via dry-run-free stubs.
	testsupport.FakeTool(t, "gh", testsupport.ToolBehavior{})

	if err := installer.Uninstall(context.Background(), false); err != nil {
		t.Fatalf("Uninstall(deleteDir=false) error = %v", err)
	}
	if _, err := os.Stat(installer.InstallDir); err != nil {
		t.Errorf("install directory removed despite deleteDir=false: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installer.InstallDir, "config.sh")); err != nil {
		t.Errorf("config.sh missing after deleteDir=false: %v", err)
	}
}
