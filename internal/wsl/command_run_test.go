package wsl

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestRunBashAsRootUsesRootFlag(t *testing.T) {
	r, lines := testsupport.RecordingRunner()

	if err := RunBashAsRoot(context.Background(), r, "Ubuntu", "apt-get update"); err != nil {
		t.Fatalf("RunBashAsRoot() error = %v", err)
	}
	got := strings.Join(lines(), " ")
	if !strings.Contains(got, "wsl.exe -d Ubuntu -u root bash -c apt-get update") {
		t.Errorf("RunBashAsRoot() echoed %q, want -u root invocation", got)
	}
}

func TestRunSudoPrefixesSudo(t *testing.T) {
	r, lines := testsupport.RecordingRunner()

	if err := RunSudo(context.Background(), r, "Ubuntu", "apt-get", "update"); err != nil {
		t.Fatalf("RunSudo() error = %v", err)
	}
	got := strings.Join(lines(), " ")
	if !strings.Contains(got, "wsl.exe -d Ubuntu -e sudo apt-get update") {
		t.Errorf("RunSudo() echoed %q, want sudo-prefixed invocation", got)
	}
}

func TestCheckCommandFound(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{})
	r := newTestRunner(t, false)

	if err := CheckCommand(context.Background(), r, "Ubuntu", "gcc"); err != nil {
		t.Errorf("CheckCommand() error = %v, want nil", err)
	}
}

func TestCheckCommandMissing(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	r := newTestRunner(t, false)

	err := CheckCommand(context.Background(), r, "Ubuntu", "gcc")
	if err == nil || !strings.Contains(err.Error(), "gcc not found") {
		t.Errorf("CheckCommand() error = %v, want 'gcc not found'", err)
	}
}
