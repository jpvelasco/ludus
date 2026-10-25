package output

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// captureRealStdout swaps os.Stdout for a temp file so that Install captures it
// as the "real" stdout, then returns what was written there after fn runs.
func captureRealStdout(t *testing.T, fn func()) string {
	t.Helper()

	tmp, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	orig := os.Stdout
	os.Stdout = tmp
	t.Cleanup(func() { os.Stdout = orig })

	fn()

	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func TestRedirectMasksStdout(t *testing.T) {
	out := captureRealStdout(t, func() {
		red, err := Install()
		if err != nil {
			t.Fatalf("Install: %v", err)
		}
		fmt.Println("image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/ludus:latest")
		red.Close()
	})

	if strings.Contains(out, "123456789012") {
		t.Errorf("expected account ID to be masked, got: %q", out)
	}
	if !strings.Contains(out, "************.dkr.ecr.us-east-1.amazonaws.com") {
		t.Errorf("expected masked ECR host, got: %q", out)
	}
}

func TestRedirectPreservesLongLines(t *testing.T) {
	// A line well over bufio.Scanner's default 64 KB token cap, proving the
	// drain loop does not truncate. The ARN at the end must survive and be masked.
	long := strings.Repeat("x", 100*1024)
	out := captureRealStdout(t, func() {
		red, err := Install()
		if err != nil {
			t.Fatalf("Install: %v", err)
		}
		fmt.Println(long + " arn:aws:gamelift:us-east-1:123456789012:fleet/x")
		red.Close()
	})

	if len(out) < 100*1024 {
		t.Errorf("long line truncated: got %d bytes", len(out))
	}
	if strings.Contains(out, ":123456789012:") {
		t.Errorf("expected account ID in long line to be masked, got tail: %q", out[len(out)-80:])
	}
}

func TestRedirectFlushesUnterminatedChunk(t *testing.T) {
	out := captureRealStdout(t, func() {
		red, err := Install()
		if err != nil {
			t.Fatalf("Install: %v", err)
		}
		fmt.Print("no newline: 123456789012.dkr.ecr.us-east-1.amazonaws.com")
		red.Close()
	})

	if !strings.Contains(out, "************.dkr.ecr") {
		t.Errorf("expected unterminated chunk flushed and masked, got: %q", out)
	}
}

func TestRedirectCloseIdempotent(t *testing.T) {
	captureRealStdout(t, func() {
		red, err := Install()
		if err != nil {
			t.Fatalf("Install: %v", err)
		}
		red.Close()
		red.Close() // must not panic or block
	})

	var nilRed *Redirect
	nilRed.Close() // must be a no-op
}

// captureStdoutFile points os.Stdout at a fresh temp file and returns its
// path, for tests that must inspect redirected output while the redirect is
// still open — before Close flushes.
func captureStdoutFile(t *testing.T) string {
	t.Helper()
	tmp, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	orig := os.Stdout
	os.Stdout = tmp
	t.Cleanup(func() { _ = tmp.Close(); os.Stdout = orig })
	return tmp.Name()
}

// pollStdoutContains waits until the captured stdout file contains want.
func pollStdoutContains(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), want) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in captured stdout", want)
}

// TestRedirectForwardsPromptWithoutNewline pins the #709 contract: an
// interactive prompt ("Continue? [y/N]: ") carries no trailing newline, and
// the process blocks on stdin right after writing it. The prompt must reach
// the terminal immediately, not only when Close flushes.
func TestRedirectForwardsPromptWithoutNewline(t *testing.T) {
	path := captureStdoutFile(t)

	red, err := Install()
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	fmt.Print("Continue? [y/N]: ")

	pollStdoutContains(t, path, "Continue? [y/N]: ")
	red.Close()
}

// TestRedirectPromptForwardKeepsMaskBoundary pins the masking invariant for
// #709's early forwarding through the redirect: an account ID split across
// writes around a forwarded prompt must never reach the terminal unmasked.
func TestRedirectPromptForwardKeepsMaskBoundary(t *testing.T) {
	path := captureStdoutFile(t)

	red, err := Install()
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// The prompt is forwarded as soon as it arrives.
	fmt.Print("Enter registry: ")
	pollStdoutContains(t, path, "Enter registry: ")

	// The account ID arrives with no newline and no prompt terminator: it
	// stays buffered at any point in time — the mask must not leak it early.
	fmt.Print("123456789012")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "123456789012") {
		t.Fatalf("account ID forwarded before its line completed: %q", data)
	}

	// The ECR host completes the line: the ID appears masked only.
	fmt.Print(".dkr.ecr.us-east-1.amazonaws.com/server\n")
	pollStdoutContains(t, path, "************.dkr.ecr.us-east-1.amazonaws.com/server")
	red.Close()

	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(final), "123456789012") {
		t.Errorf("split account ID leaked unmasked through the redirect: %q", final)
	}
}
