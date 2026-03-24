package runner

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestEnviron(t *testing.T) {
	t.Run("nil when Env is empty", func(t *testing.T) {
		r := NewRunner(false, false)
		if env := r.environ(); env != nil {
			t.Errorf("expected nil, got %v", env)
		}
	})

	t.Run("adds new variables", func(t *testing.T) {
		r := NewRunner(false, false)
		r.Env = []string{"LUDUS_TEST_VAR=hello"}
		env := r.environ()
		if env == nil {
			t.Fatal("expected non-nil env")
		}
		found := false
		for _, kv := range env {
			if kv == "LUDUS_TEST_VAR=hello" {
				found = true
				break
			}
		}
		if !found {
			t.Error("LUDUS_TEST_VAR=hello not found in merged env")
		}
	})

	t.Run("overrides existing variables", func(t *testing.T) {
		r := NewRunner(false, false)
		// PATH is virtually guaranteed to exist in the parent environment.
		r.Env = []string{"PATH=/override/path"}
		env := r.environ()
		if env == nil {
			t.Fatal("expected non-nil env")
		}
		count := 0
		for _, kv := range env {
			if len(kv) >= 5 && kv[:5] == "PATH=" {
				count++
				if kv != "PATH=/override/path" {
					t.Errorf("expected PATH=/override/path, got %s", kv)
				}
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 PATH entry, got %d", count)
		}
	})

	t.Run("preserves parent env alongside overrides", func(t *testing.T) {
		r := NewRunner(false, false)
		r.Env = []string{"LUDUS_TEST_ONLY=1"}
		env := r.environ()
		// The merged env should contain at least the parent env entries plus
		// the new variable.
		if len(env) < 2 {
			t.Errorf("merged env too small: %d entries", len(env))
		}
	})
}

func TestRun_Success(t *testing.T) {
	r := NewRunner(false, false)
	ctx := context.Background()
	err := r.Run(ctx, "go", "version")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestRun_Failure(t *testing.T) {
	r := NewRunner(false, false)
	ctx := context.Background()
	err := r.Run(ctx, "nonexistent-ludus-command-xyz")
	if err == nil {
		t.Error("expected non-nil error, got nil")
	}
}

func TestRunOutput_CapturesStdout(t *testing.T) {
	r := NewRunner(false, false)
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()
	out, err := r.RunOutput(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "go version") {
		t.Errorf("expected output to contain 'go version', got: %s", outStr)
	}
}

func TestDryRun_DoesNotExecute(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	// Use a command that would fail if executed
	err := r.Run(ctx, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}

	// Verify the command was printed
	output := stdout.String()
	if !strings.Contains(output, "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected stdout to contain '+ nonexistent-ludus-command-xyz', got: %s", output)
	}
}

func TestVerbose_PrintsCommand(t *testing.T) {
	r := NewRunner(true, false) // Verbose=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	err := r.Run(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Verify the command was printed
	output := stdout.String()
	if !strings.Contains(output, "+ go version") {
		t.Errorf("expected stdout to contain '+ go version', got: %s", output)
	}
}

func TestRunInDir_Success(t *testing.T) {
	r := NewRunner(false, false)
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	// Run "go version" in a temp directory
	tmpDir := t.TempDir()
	err := r.RunInDir(ctx, tmpDir, "go", "version")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestRunInDir_DryRun(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	tmpDir := t.TempDir()
	err := r.RunInDir(ctx, tmpDir, "nonexistent-ludus-command-xyz", "arg1")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "+ cd") {
		t.Errorf("expected dry-run output to contain '+ cd', got: %s", output)
	}
	if !strings.Contains(output, "nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output to contain command name, got: %s", output)
	}
}

func TestRunWithStdin_Success(t *testing.T) {
	r := NewRunner(false, false)
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	input := strings.NewReader("hello from stdin")
	err := r.RunWithStdin(ctx, input, "go", "version")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestRunWithStdin_DryRun(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	input := strings.NewReader("unused input")
	err := r.RunWithStdin(ctx, input, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output to contain command, got: %s", output)
	}
}

func TestRunQuiet_SuppressesStdout(t *testing.T) {
	r := NewRunner(false, false) // not verbose
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	err := r.RunQuiet(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout in quiet mode, got: %s", stdout.String())
	}
}

func TestRunQuiet_ShowsStdoutWhenVerbose(t *testing.T) {
	r := NewRunner(true, false) // verbose
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	err := r.RunQuiet(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "+ go version") {
		t.Errorf("expected verbose prefix, got: %s", output)
	}
	if !strings.Contains(output, "go version go") {
		t.Errorf("expected command output in verbose mode, got: %s", output)
	}
}

func TestRunQuiet_DryRun(t *testing.T) {
	r := NewRunner(false, true) // dry-run
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	err := r.RunQuiet(ctx, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output, got: %s", stdout.String())
	}
}

func TestRunQuietWithStdin_SuppressesStdout(t *testing.T) {
	r := NewRunner(false, false) // not verbose
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	input := strings.NewReader("hello")
	err := r.RunQuietWithStdin(ctx, input, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout in quiet mode, got: %s", stdout.String())
	}
}

func TestRunQuietWithStdin_DryRun(t *testing.T) {
	r := NewRunner(false, true) // dry-run
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	input := strings.NewReader("unused")
	err := r.RunQuietWithStdin(ctx, input, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output, got: %s", stdout.String())
	}
}

func TestRunOutput_DryRun(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	out, err := r.RunOutput(ctx, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}
	if string(out) != "(dry-run)" {
		t.Errorf("expected '(dry-run)', got %q", string(out))
	}
}
