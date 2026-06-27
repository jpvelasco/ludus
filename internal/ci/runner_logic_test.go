package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigArgs(t *testing.T) {
	ri := &RunnerInstaller{
		Repo:   "jpvelasco/ludus",
		Labels: "self-hosted,linux,x64",
		Name:   "ludus-runner-1",
	}
	args := ri.configArgs("tok-abc123")
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--url https://github.com/jpvelasco/ludus",
		"--token tok-abc123",
		"--labels self-hosted,linux,x64",
		"--name ludus-runner-1",
		"--unattended",
		"--replace",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("configArgs missing %q\ngot: %s", want, joined)
		}
	}
}

func TestWriteWorkflow_MkdirError(t *testing.T) {
	// A path whose parent directory is actually a file cannot be created.
	dir := t.TempDir()
	fileAsParent := filepath.Join(dir, "blocker")
	if err := os.WriteFile(fileAsParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(fileAsParent, "workflow", "ci.yml")
	if err := WriteWorkflow(target, "name: CI\n"); err == nil {
		t.Error("expected error when parent path is a file")
	}
}
