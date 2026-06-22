package output

import (
	"fmt"
	"os"
	"strings"
	"testing"
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
