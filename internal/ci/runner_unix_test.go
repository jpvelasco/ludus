//go:build linux

package ci

import (
	"bytes"
	"context"
	"io"
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
	stdout := captureRunnerStdout(t, func() error {
		return installer.Uninstall(context.Background(), true)
	})
	if err := stdout.err; err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	for _, want := range []string{"svc.sh stop", "svc.sh uninstall", "remove-token", "config.sh remove"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, output.String())
		}
	}
	// #707: the preview reports the commands without executing them, so the
	// installation must survive a dry-run uninstall.
	if _, err := os.Stat(installer.InstallDir); err != nil {
		t.Errorf("install directory deleted by dry-run Uninstall(deleteDir=true): %v", err)
	}
	if !strings.Contains(stdout.text, "Dry run: would delete install directory") {
		t.Errorf("dry-run output missing the deletion preview:\n%s", stdout.text)
	}
}

// captureRunnerStdout runs fn while capturing os.Stdout (status messages)
// alongside the runner's echoed buffer, and returns the captured text.
type stdoutResult struct {
	text string
	err  error
}

func captureRunnerStdout(t *testing.T, fn func() error) stdoutResult {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previous })
	runErr := fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	os.Stdout = previous
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return stdoutResult{text: string(data), err: runErr}
}
