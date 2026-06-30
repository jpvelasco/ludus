//go:build !windows

package ci

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func dryRunInstaller(t *testing.T) (*RunnerInstaller, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	return &RunnerInstaller{
		Runner:     &runner.Runner{DryRun: true, Stdout: &output, Stderr: &output},
		InstallDir: t.TempDir(),
		Labels:     "self-hosted,linux,x64",
		Name:       "test-runner",
		Repo:       "owner/repo",
	}, &output
}

func TestRunnerInstallerInstallDryRun(t *testing.T) {
	installer, output := dryRunInstaller(t)
	if err := installer.Install(context.Background()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for _, want := range []string{"gh api", "curl -o", "tar xzf", "config.sh", "--unattended"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRunnerInstallerFinishInstallServiceDryRun(t *testing.T) {
	installer, output := dryRunInstaller(t)
	installer.Service = true
	if err := installer.finishInstall(context.Background(), installer.InstallDir); err != nil {
		t.Fatalf("finishInstall() error = %v", err)
	}
	for _, want := range []string{"svc.sh install", "svc.sh start"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRunnerInstallerStatus(t *testing.T) {
	tests := []struct {
		name      string
		config    bool
		service   bool
		want      string
		wantTrace string
	}{
		{name: "not installed", want: "not installed"},
		{name: "process", config: true, want: "running (process)", wantTrace: "pgrep -f Runner.Listener"},
		{name: "service", config: true, service: true, want: "running (systemd)", wantTrace: "svc.sh status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, output := dryRunInstaller(t)
			writeRunnerMarkers(t, installer.InstallDir, tt.config, tt.service)
			got, err := installer.Status(context.Background())
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
			if tt.wantTrace != "" && !strings.Contains(output.String(), tt.wantTrace) {
				t.Errorf("dry-run output missing %q:\n%s", tt.wantTrace, output.String())
			}
		})
	}
}

func writeRunnerMarkers(t *testing.T, dir string, config, service bool) {
	t.Helper()
	for name, enabled := range map[string]bool{"config.sh": config, "svc.sh": service} {
		if enabled {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRunnerInstallerUninstallDryRun(t *testing.T) {
	installer, output := dryRunInstaller(t)
	writeRunnerMarkers(t, installer.InstallDir, true, true)
	if err := installer.Uninstall(context.Background(), true); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	for _, want := range []string{"svc.sh stop", "svc.sh uninstall", "remove-token", "config.sh remove"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, output.String())
		}
	}
	if _, err := os.Stat(installer.InstallDir); !os.IsNotExist(err) {
		t.Errorf("install directory still exists after Uninstall(deleteDir=true): %v", err)
	}
}
