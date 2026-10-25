package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestMaskingWriterMasksAcrossWrites pins the #554 contract: account IDs are
// masked even when a line arrives split across several Write calls (as with
// child-process stderr), and the forwarded text is otherwise byte-identical.
func TestMaskingWriterMasksAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	mw := NewMaskingWriter(&buf)

	_, _ = mw.Write([]byte("The push refers to repository [123456789012"))
	_, _ = mw.Write([]byte(".dkr.ecr.us-east-1.amazonaws.com/ludus-server]\n"))

	got := buf.String()
	if strings.Contains(got, "123456789012") {
		t.Errorf("account ID not masked across split writes: %q", got)
	}
	if !strings.Contains(got, "************.dkr.ecr.us-east-1.amazonaws.com/ludus-server]\n") {
		t.Errorf("unexpected masked output: %q", got)
	}
}

func TestMaskingWriterBuffersPartialLine(t *testing.T) {
	var buf bytes.Buffer
	mw := NewMaskingWriter(&buf)

	_, _ = mw.Write([]byte("pushing 123456789012.dkr.ecr.us-east-1.amazonaws.com/x"))
	if buf.Len() != 0 {
		t.Errorf("partial line leaked before newline: %q", buf.String())
	}

	mw.Flush()
	if strings.Contains(buf.String(), "123456789012") {
		t.Errorf("flushed partial line not masked: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "************.dkr.ecr.us-east-1.amazonaws.com/x") {
		t.Errorf("unexpected flushed output: %q", buf.String())
	}
}

// TestMaskingWriterForwardsPromptTerminatedPartial pins the #709 contract: an
// interactive prompt (setup questions, purge confirmations) ends with ": " and
// no newline, so waiting for a newline would hide the question while the child
// blocks reading stdin. Prompt-terminated partial lines are forwarded at once.
func TestMaskingWriterForwardsPromptTerminatedPartial(t *testing.T) {
	tests := []struct {
		name       string
		prompt     string
		rest       string
		wantPrompt string
	}{
		{
			name:       "colon-space prompt",
			prompt:     "Enter AWS region [us-east-1]: ",
			rest:       "us-east-1\n",
			wantPrompt: "Enter AWS region [us-east-1]: ",
		},
		{
			name:       "confirmation prompt",
			prompt:     "Continue? [y/N]: ",
			rest:       "y\n",
			wantPrompt: "Continue? [y/N]: ",
		},
		{
			name:       "question-space prompt",
			prompt:     "Proceed? ",
			rest:       "yes\n",
			wantPrompt: "Proceed? ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			mw := NewMaskingWriter(&buf)

			_, _ = mw.Write([]byte(tt.prompt))
			if got := buf.String(); got != tt.wantPrompt {
				t.Fatalf("prompt not forwarded without a newline: got %q, want %q", got, tt.wantPrompt)
			}

			// The rest of the line is still line-buffered and masked normally.
			_, _ = mw.Write([]byte(tt.rest))
			if want := tt.wantPrompt + tt.rest; buf.String() != want {
				t.Errorf("continuation = %q, want %q", buf.String(), want)
			}
		})
	}
}

// TestMaskingWriterWriteErrorPropagates pins that a failing destination
// surfaces its error from Write for both complete lines and prompt-terminated
// partials, while still reporting the number of input bytes consumed.
func TestMaskingWriterWriteErrorPropagates(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "complete line", in: "line 123456789012.dkr.ecr.us-east-1.amazonaws.com/x\n"},
		{name: "prompt-terminated partial", in: "Region [us-east-1]: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewMaskingWriter(errWriter{})
			n, err := mw.Write([]byte(tt.in))
			if err == nil {
				t.Fatal("expected the destination error to propagate")
			}
			if n != len(tt.in) {
				t.Errorf("Write reported %d bytes, want %d", n, len(tt.in))
			}
		})
	}
}

// errWriter is a destination that always fails.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// TestMaskingWriterPromptForwardKeepsMaskBoundary pins the masking invariant
// for #709's early forwarding: an account ID split across writes around a
// prompt terminator must never reach the destination unmasked.
func TestMaskingWriterPromptForwardKeepsMaskBoundary(t *testing.T) {
	var buf bytes.Buffer
	mw := NewMaskingWriter(&buf)

	_, _ = mw.Write([]byte("Enter registry: "))
	if !strings.Contains(buf.String(), "Enter registry: ") {
		t.Fatalf("prompt withheld: %q", buf.String())
	}

	// The account ID arrives in its own write, without a prompt terminator
	// or newline: it must stay buffered, invisible.
	_, _ = mw.Write([]byte("123456789012"))
	if strings.Contains(buf.String(), "123456789012") {
		t.Fatalf("account ID forwarded before its line completed: %q", buf.String())
	}

	// The ECR host completes the line; the ID must only ever appear masked.
	_, _ = mw.Write([]byte(".dkr.ecr.us-east-1.amazonaws.com/server\n"))
	got := buf.String()
	if strings.Contains(got, "123456789012") {
		t.Errorf("split account ID leaked unmasked: %q", got)
	}
	if !strings.Contains(got, "************.dkr.ecr.us-east-1.amazonaws.com/server") {
		t.Errorf("expected masked ECR host, got: %q", got)
	}
}
