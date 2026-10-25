package mcp

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/jpvelasco/ludus/internal/runner"
)

// syncBuffer is a thread-safe bytes.Buffer implementing io.Writer.
// Used as Runner.Stdout/Stderr for per-build output capture. When sink is set
// (a per-build log file), writes are mirrored there so MCP builds are persisted
// to disk the same way CLI builds are.
type syncBuffer struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	sink io.Writer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.sink != nil {
		_, _ = sb.sink.Write(p)
	}
	return sb.buf.Write(p)
}

// Len returns the number of bytes in the buffer.
func (sb *syncBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

// String returns the buffered output.
func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// tailLines returns the last n lines from the buffer.
func (sb *syncBuffer) tailLines(n int) string {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	data := sb.buf.String()
	lines := strings.Split(data, "\n")

	// Trim trailing empty line from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// toolOutput is the per-operation output sink for one MCP tool invocation.
// The MCP SDK dispatches tool calls concurrently (jsonrpc2.Async), so a
// handler must never swap the process-global os.Stdout/os.Stderr to capture
// output: overlapping handlers would steal each other's prints and restore
// each other's closed pipes (#725). Instead every invocation owns a
// toolOutput; subprocess streams are wired to it via newCaptureRunner, and
// handler-owned status prints route through its Printf/Println methods.
// The tool response assembles from String().
type toolOutput struct {
	buf *syncBuffer
}

// newToolOutput returns an empty per-invocation capture.
func newToolOutput() *toolOutput {
	return &toolOutput{buf: &syncBuffer{}}
}

// Write routes writes into the capture; toolOutput is an io.Writer.
func (o *toolOutput) Write(p []byte) (int, error) { return o.buf.Write(p) }

// Printf routes a handler-owned status line into the capture.
func (o *toolOutput) Printf(format string, a ...any) { fmt.Fprintf(o.buf, format, a...) }

// Println routes a handler-owned status line into the capture.
func (o *toolOutput) Println(a ...any) { fmt.Fprintln(o.buf, a...) }

// String returns everything captured for this invocation.
func (o *toolOutput) String() string { return o.buf.String() }

// newCaptureRunner returns a runner whose subprocess output — stdout, stderr,
// and verbose/dry-run command echoes — routes into w instead of the process
// streams, so a tool response carries the child's build logs and error
// details instead of them leaking to the server's stderr (#747). dryRun is
// the EFFECTIVE dry-run for this operation: callers pass deployToolDryRun's
// result (sync handlers) or an already-resolved value (async starters, which
// must resolve before spawning so the build goroutine never reads globals).
func newCaptureRunner(w io.Writer, dryRun bool) *runner.Runner {
	return &runner.Runner{
		Stdout:  w,
		Stderr:  w,
		Verbose: true,
		DryRun:  dryRun,
	}
}
