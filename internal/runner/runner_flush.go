package runner

import "io"

// flusher is implemented by buffering writers (e.g. the account-ID masking
// writer wired up by cmd/globals) that hold an unterminated trailing line
// until Flush is called. Without draining them at command completion, a child
// that ends its output without a trailing newline would lose that message
// from both the terminal and the build-log sink (#711).
type flusher interface {
	Flush()
}

// flush drains any buffering writers attached to this runner. Every command
// execution method calls it via defer, so the trailing output of a child is
// delivered on all completion paths: success, failure, and cancellation.
func (r *Runner) flush() {
	flushWriter(r.Stdout)
	flushWriter(r.Stderr)
}

// flushWriter flushes w when it is a buffering writer; plain writers are
// untouched.
func flushWriter(w io.Writer) {
	if f, ok := w.(flusher); ok {
		f.Flush()
	}
}
