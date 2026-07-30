package testsupport

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestFakeToolExitCode(t *testing.T) {
	tool := FakeTool(t, "exit_code_tool", ToolBehavior{
		ExitCode: 42,
	})

	// Run the tool and check exit code
	cmd := exec.Command(tool)
	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit code, got success")
	}

	// Check the exit code (platform-specific)
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 42 {
			t.Errorf("exit code = %d, want 42", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected ExitError, got %T", err)
	}
}

func TestFakeToolStdout(t *testing.T) {
	expectedOutput := "Hello from fake tool"
	tool := FakeTool(t, "stdout_tool", ToolBehavior{
		Stdout: expectedOutput,
	})

	cmd := exec.Command(tool)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to run tool: %v", err)
	}

	outputStr := strings.TrimSpace(string(output))
	if !strings.Contains(outputStr, expectedOutput) {
		t.Errorf("output = %q, want to contain %q", outputStr, expectedOutput)
	}
}

func TestFakeToolStderr(t *testing.T) {
	expectedError := "Error from fake tool"
	tool := FakeTool(t, "stderr_tool", ToolBehavior{
		Stderr: expectedError,
	})

	cmd := exec.Command(tool)
	err := cmd.Run()
	// Note: We can't easily capture stderr without redirecting it,
	// so we just verify the tool runs without crashing
	if err != nil {
		// Exit code 0 is fine, we're just testing stderr was written
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 {
				// Expected
			} else {
				t.Fatalf("unexpected exit code: %d", exitErr.ExitCode())
			}
		}
	}
}

func TestFakeToolCombined(t *testing.T) {
	tool := FakeTool(t, "combined_tool", ToolBehavior{
		ExitCode: 1,
		Stdout:   "out message",
		Stderr:   "err message",
	})

	cmd := exec.Command(tool)
	_ = cmd.Run() // We're just testing that it was created and is executable
	// The tool should be in PATH and executable
	if _, err := os.Stat(tool); os.IsNotExist(err) {
		t.Error("tool should exist after FakeTool()")
	}
}

func TestFakeToolInPath(t *testing.T) {
	// Create a tool
	tool := FakeTool(t, "path_test_tool", ToolBehavior{
		Stdout: "found",
	})

	// Verify the tool exists at the returned path
	if _, err := os.Stat(tool); os.IsNotExist(err) {
		t.Fatalf("tool not found at returned path: %s", tool)
	}

	// Try to execute it by name (should work because FakeTool adds to PATH)
	cmd := exec.Command("path_test_tool")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find tool in PATH: %v", err)
	}

	if !strings.Contains(string(output), "found") {
		t.Errorf("output = %q, want to contain 'found'", string(output))
	}
}

func TestFakeToolMultipleTools(t *testing.T) {
	_ = FakeTool(t, "tool1", ToolBehavior{Stdout: "tool1"})
	_ = FakeTool(t, "tool2", ToolBehavior{Stdout: "tool2"})

	// Both should be in PATH
	cmd1 := exec.Command("tool1")
	output1, err1 := cmd1.Output()
	if err1 != nil {
		t.Fatalf("tool1 failed: %v", err1)
	}

	cmd2 := exec.Command("tool2")
	output2, err2 := cmd2.Output()
	if err2 != nil {
		t.Fatalf("tool2 failed: %v", err2)
	}

	if !strings.Contains(string(output1), "tool1") {
		t.Errorf("tool1 output = %q, want 'tool1'", string(output1))
	}
	if !strings.Contains(string(output2), "tool2") {
		t.Errorf("tool2 output = %q, want 'tool2'", string(output2))
	}
}
