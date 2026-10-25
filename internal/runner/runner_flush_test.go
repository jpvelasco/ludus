package runner

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// flushRecorder is a writer that buffers output and counts Flush calls, like
// the account-ID masking writer wired into runners by cmd/globals.
type flushRecorder struct {
	bytes.Buffer
	flushes int
}

func (f *flushRecorder) Flush() { f.flushes++ }

// TestRunnerFlushesBufferingWritersOnCompletion pins the #711 contract: a
// runner must drain buffering writers (MaskingWriter) when a command finishes
// on every execution method, so a child ending its output without a trailing
// newline does not lose that message from the terminal or the log sink.
func TestRunnerFlushesBufferingWritersOnCompletion(t *testing.T) {
	tests := []struct {
		name    string
		run     func(ctx context.Context, r *Runner) error
		wantErr bool
	}{
		{name: "run success", run: func(ctx context.Context, r *Runner) error {
			return r.Run(ctx, "go", "version")
		}},
		{name: "run failure", wantErr: true, run: func(ctx context.Context, r *Runner) error {
			return r.Run(ctx, "nonexistent-ludus-command-xyz")
		}},
		{name: "run with stdin", run: func(ctx context.Context, r *Runner) error {
			return r.RunWithStdin(ctx, strings.NewReader("in"), "go", "version")
		}},
		{name: "run in dir", run: func(ctx context.Context, r *Runner) error {
			return r.RunInDir(ctx, ".", "go", "version")
		}},
		{name: "run output", run: func(ctx context.Context, r *Runner) error {
			_, err := r.RunOutput(ctx, "go", "version")
			return err
		}},
		{name: "run quiet", run: func(ctx context.Context, r *Runner) error {
			return r.RunQuiet(ctx, "go", "version")
		}},
		{name: "run quiet err failure", wantErr: true, run: func(ctx context.Context, r *Runner) error {
			return r.RunQuietErr(ctx, "nonexistent-ludus-command-xyz")
		}},
		{name: "run quiet with stdin", run: func(ctx context.Context, r *Runner) error {
			return r.RunQuietWithStdin(ctx, strings.NewReader("in"), "go", "version")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRunnerFlushedOnce(t, tt.run, tt.wantErr)
		})
	}
}

// assertRunnerFlushedOnce runs one command method through a runner wired to
// flush recorders and asserts both writers were drained exactly once.
func assertRunnerFlushedOnce(t *testing.T, run func(ctx context.Context, r *Runner) error, wantErr bool) {
	t.Helper()
	stdout, stderr := &flushRecorder{}, &flushRecorder{}
	r := &Runner{Stdout: stdout, Stderr: stderr}

	err := run(context.Background(), r)
	if wantErr != (err != nil) {
		t.Fatalf("command error = %v, wantErr %v", err, wantErr)
	}
	if stdout.flushes != 1 {
		t.Errorf("stdout flushed %d times, want exactly 1", stdout.flushes)
	}
	if stderr.flushes != 1 {
		t.Errorf("stderr flushed %d times, want exactly 1", stderr.flushes)
	}
}

// TestRunnerFlushesBufferingWritersOnCancellation covers the cancellation
// path: a context cancelled before the command even starts must still drain
// the buffering writers.
func TestRunnerFlushesBufferingWritersOnCancellation(t *testing.T) {
	stdout, stderr := &flushRecorder{}, &flushRecorder{}
	r := &Runner{Stdout: stdout, Stderr: stderr}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Run(ctx, "go", "version"); err == nil {
		t.Fatal("expected an error from a pre-cancelled context")
	}
	if stdout.flushes != 1 || stderr.flushes != 1 {
		t.Errorf("flushes after cancellation: stdout=%d stderr=%d, want 1 and 1", stdout.flushes, stderr.flushes)
	}
}

// TestRunnerFlushIsNoOpForPlainWriters pins that writers without buffering
// (plain buffers, MultiWriter) are untouched by the flush step.
func TestRunnerFlushIsNoOpForPlainWriters(t *testing.T) {
	r := &Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	r.flush()
}
