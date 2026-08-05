package ci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// ciDryRunInstaller builds a RunnerInstaller whose runner is in dry-run mode
// (echoes commands, executes nothing) and captures echoed output. Mirrors the
// dryRunInstaller helper in runner_unix_test.go so these tests run on every OS.
func ciDryRunInstaller(t *testing.T) (*RunnerInstaller, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	return &RunnerInstaller{
		Runner:     &runner.Runner{DryRun: true, Stdout: &out, Stderr: &out},
		InstallDir: t.TempDir(),
		Labels:     "self-hosted,linux,x64",
		Name:       "test-runner",
		Repo:       "owner/repo",
	}, &out
}

// realRunner returns a non-dry-run runner that sends output to buffers, so
// tests that stub external tools on PATH execute those stubs hermetically.
func realRunner() *runner.Runner {
	return &runner.Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
}

// writeArgFailingTool writes a PATH-injected stub named name that exits 1 when
// the argument at 1-based index argNum equals failValue; otherwise it prints
// stdout and exits 0.
func writeArgFailingTool(t *testing.T, name string, argNum int, failValue, stdout string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if runtime.GOOS == "windows" {
		script := "@echo off\n" +
			fmt.Sprintf("if \"%%%d\"==\"%s\" exit /b 1\n", argNum, failValue) +
			"echo " + stdout + "\n"
		if err := os.WriteFile(filepath.Join(dir, name+".bat"), []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	script := "#!/bin/sh\n" +
		fmt.Sprintf("[ \"$%d\" = \"%s\" ] && exit 1\n", argNum, failValue) +
		fmt.Sprintf("printf '%%s\\n' %q\n", stdout)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerInstallerInstallMetadata(t *testing.T) {
	ri, _ := ciDryRunInstaller(t)
	token, version, err := ri.installMetadata(context.Background())
	if err != nil {
		t.Fatalf("installMetadata() error = %v", err)
	}
	if token == "" || version == "" {
		t.Errorf("installMetadata() = (%q, %q), want non-empty values", token, version)
	}
}

func TestRunnerInstallerInstallMetadataTokenError(t *testing.T) {
	testsupport.FakeTool(t, "gh", testsupport.ToolBehavior{ExitCode: 1})
	ri := &RunnerInstaller{Runner: realRunner(), Repo: "owner/repo"}
	_, _, err := ri.installMetadata(context.Background())
	if err == nil || !strings.Contains(err.Error(), "getting registration token") {
		t.Fatalf("installMetadata() error = %v, want registration token error", err)
	}
}

func TestRunnerInstallerInstallMetadataVersionError(t *testing.T) {
	writeArgFailingTool(t, "gh", 2, "repos/actions/runner/releases/latest", "some-token")
	ri := &RunnerInstaller{Runner: realRunner(), Repo: "owner/repo"}
	_, _, err := ri.installMetadata(context.Background())
	if err == nil || !strings.Contains(err.Error(), "getting runner version") {
		t.Fatalf("installMetadata() error = %v, want runner version error", err)
	}
}

func TestRunnerInstallerRegistrationToken(t *testing.T) {
	ri, output := ciDryRunInstaller(t)
	token, err := ri.registrationToken(context.Background())
	if err != nil {
		t.Fatalf("registrationToken() error = %v", err)
	}
	if token != "(dry-run)" {
		t.Errorf("registrationToken() = %q, want %q", token, "(dry-run)")
	}
	if !strings.Contains(output.String(), "gh api") {
		t.Errorf("dry-run output missing 'gh api':\n%s", output.String())
	}
}

func TestRunnerInstallerRegistrationTokenError(t *testing.T) {
	testsupport.FakeTool(t, "gh", testsupport.ToolBehavior{ExitCode: 1})
	ri := &RunnerInstaller{Runner: realRunner()}
	_, err := ri.registrationToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "getting registration token") {
		t.Fatalf("registrationToken() error = %v, want wrapped error", err)
	}
}

func TestRunnerInstallerLatestRunnerVersion(t *testing.T) {
	ri, output := ciDryRunInstaller(t)
	version, err := ri.latestRunnerVersion(context.Background())
	if err != nil {
		t.Fatalf("latestRunnerVersion() error = %v", err)
	}
	if version != "(dry-run)" {
		t.Errorf("latestRunnerVersion() = %q, want %q", version, "(dry-run)")
	}
	if !strings.Contains(output.String(), "gh api") {
		t.Errorf("dry-run output missing 'gh api':\n%s", output.String())
	}
}

func TestRunnerInstallerLatestRunnerVersionError(t *testing.T) {
	testsupport.FakeTool(t, "gh", testsupport.ToolBehavior{ExitCode: 1})
	ri := &RunnerInstaller{Runner: realRunner()}
	_, err := ri.latestRunnerVersion(context.Background())
	if err == nil || !strings.Contains(err.Error(), "getting runner version") {
		t.Fatalf("latestRunnerVersion() error = %v, want wrapped error", err)
	}
}

func TestRunnerInstallerDownloadRunner(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{version: "v2.300.0", want: "actions-runner-linux-x64-2.300.0.tar.gz"},
		{version: "2.300.0", want: "actions-runner-linux-x64-2.300.0.tar.gz"},
	}
	for _, tt := range tests {
		ri, output := ciDryRunInstaller(t)
		got, err := ri.downloadRunner(context.Background(), ri.InstallDir, tt.version)
		if err != nil {
			t.Fatalf("downloadRunner(%q) error = %v", tt.version, err)
		}
		if got != tt.want {
			t.Errorf("downloadRunner(%q) = %q, want %q", tt.version, got, tt.want)
		}
		wantURL := "https://github.com/actions/runner/releases/download/" + tt.version + "/" + tt.want
		if !strings.Contains(output.String(), wantURL) {
			t.Errorf("dry-run output missing URL %q:\n%s", wantURL, output.String())
		}
	}
}

func TestRunnerInstallerDownloadRunnerError(t *testing.T) {
	testsupport.FakeTool(t, "curl", testsupport.ToolBehavior{ExitCode: 1})
	ri := &RunnerInstaller{Runner: realRunner()}
	_, err := ri.downloadRunner(context.Background(), t.TempDir(), "v2.300.0")
	if err == nil || !strings.Contains(err.Error(), "downloading runner") {
		t.Fatalf("downloadRunner() error = %v, want wrapped error", err)
	}
}

func TestRunnerInstallerExtractRunner(t *testing.T) {
	ri, output := ciDryRunInstaller(t)
	tarball := "actions-runner-linux-x64-2.300.0.tar.gz"
	if err := os.WriteFile(filepath.Join(ri.InstallDir, tarball), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ri.extractRunner(context.Background(), ri.InstallDir, tarball); err != nil {
		t.Fatalf("extractRunner() error = %v", err)
	}
	if !strings.Contains(output.String(), "tar xzf") {
		t.Errorf("dry-run output missing 'tar xzf':\n%s", output.String())
	}
	if _, err := os.Stat(filepath.Join(ri.InstallDir, tarball)); !os.IsNotExist(err) {
		t.Errorf("tarball still present after extractRunner(): %v", err)
	}
}

func TestRunnerInstallerExtractRunnerError(t *testing.T) {
	testsupport.FakeTool(t, "tar", testsupport.ToolBehavior{ExitCode: 1})
	ri := &RunnerInstaller{Runner: realRunner()}
	err := ri.extractRunner(context.Background(), t.TempDir(), "some.tar.gz")
	if err == nil || !strings.Contains(err.Error(), "extracting runner") {
		t.Fatalf("extractRunner() error = %v, want wrapped error", err)
	}
}

func TestRunnerInstallerConfigureRunner(t *testing.T) {
	ri, output := ciDryRunInstaller(t)
	if err := ri.configureRunner(context.Background(), ri.InstallDir, "tok-abc"); err != nil {
		t.Fatalf("configureRunner() error = %v", err)
	}
	out := output.String()
	for _, want := range []string{"config.sh", "--url https://github.com/owner/repo", "--token tok-abc", "--unattended"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestRunnerInstallerConfigureRunnerError(t *testing.T) {
	ri := &RunnerInstaller{Runner: realRunner()}
	err := ri.configureRunner(context.Background(), t.TempDir(), "tok-abc")
	if err == nil || !strings.Contains(err.Error(), "configuring runner") {
		t.Fatalf("configureRunner() error = %v, want wrapped error", err)
	}
}

func TestRunnerInstallerInstallRunnerFiles(t *testing.T) {
	ri, output := ciDryRunInstaller(t)
	if err := ri.installRunnerFiles(context.Background(), ri.InstallDir, "v2.300.0"); err != nil {
		t.Fatalf("installRunnerFiles() error = %v", err)
	}
	out := output.String()
	for _, want := range []string{"curl -o", "tar xzf"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestRunnerInstallerInstallRunnerFilesMkdirError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ri := &RunnerInstaller{Runner: realRunner()}
	err := ri.installRunnerFiles(context.Background(), filepath.Join(blocker, "sub"), "v2.300.0")
	if err == nil || !strings.Contains(err.Error(), "creating install directory") {
		t.Fatalf("installRunnerFiles() error = %v, want mkdir error", err)
	}
}

func TestRunnerInstallerInstallRunnerFilesDownloadError(t *testing.T) {
	testsupport.FakeTool(t, "curl", testsupport.ToolBehavior{ExitCode: 1})
	ri := &RunnerInstaller{Runner: realRunner()}
	err := ri.installRunnerFiles(context.Background(), t.TempDir(), "v2.300.0")
	if err == nil || !strings.Contains(err.Error(), "downloading runner") {
		t.Fatalf("installRunnerFiles() error = %v, want download error", err)
	}
}

func TestRunnerInstallerInstallRunnerFilesExtractError(t *testing.T) {
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"curl": {},
		"tar":  {ExitCode: 1},
	})
	ri := &RunnerInstaller{Runner: realRunner()}
	err := ri.installRunnerFiles(context.Background(), t.TempDir(), "v2.300.0")
	if err == nil || !strings.Contains(err.Error(), "extracting runner") {
		t.Fatalf("installRunnerFiles() error = %v, want extract error", err)
	}
}

func TestRunnerInstallerFinishInstallNoService(t *testing.T) {
	ri, _ := ciDryRunInstaller(t)
	if err := ri.finishInstall(context.Background(), ri.InstallDir); err != nil {
		t.Fatalf("finishInstall() error = %v", err)
	}
}

func TestRunnerInstallerFinishInstallService(t *testing.T) {
	ri, output := ciDryRunInstaller(t)
	ri.Service = true
	if err := ri.finishInstall(context.Background(), ri.InstallDir); err != nil {
		t.Fatalf("finishInstall() error = %v", err)
	}
	out := output.String()
	for _, want := range []string{"svc.sh install", "svc.sh start"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestRunnerInstallerFinishInstallServiceInstallError(t *testing.T) {
	testsupport.FakeTool(t, "sudo", testsupport.ToolBehavior{ExitCode: 1})
	ri := &RunnerInstaller{Runner: realRunner(), Service: true}
	err := ri.finishInstall(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "installing service") {
		t.Fatalf("finishInstall() error = %v, want installing-service error", err)
	}
}

func TestRunnerInstallerFinishInstallServiceStartError(t *testing.T) {
	writeArgFailingTool(t, "sudo", 2, "start", "")
	ri := &RunnerInstaller{Runner: realRunner(), Service: true}
	err := ri.finishInstall(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "starting service") {
		t.Fatalf("finishInstall() error = %v, want starting-service error", err)
	}
}
