package output

import (
	"io"
	"strings"
	"sync"
)

// promptTerminators are the suffixes that mark the end of an interactive
// prompt: a question a process writes and then blocks reading stdin (setup
// prompts and the --purge confirmation end with ": " and no newline). A
// purely line-buffered masker would withhold the question until a newline
// that never arrives while the process waits for input (#709).
//
// Splitting the stream at such a suffix is masking-safe: no maskable
// account-ID pattern (an ECR registry host or an ARN account field) contains
// a space, so a pattern can never straddle the point where a prompt-terminated
// partial line is forwarded.
var promptTerminators = []string{": ", "? "}

// promptTerminated reports whether the incomplete line s ends with an
// interactive prompt terminator and should therefore be forwarded immediately
// instead of buffered until a newline arrives.
func promptTerminated(s string) bool {
	for _, suffix := range promptTerminators {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// MaskingWriter wraps an io.Writer and masks AWS account IDs in every
// complete line before forwarding it. Partial lines are buffered until a
// newline arrives, Flush is called, or the partial line ends with an
// interactive prompt terminator (see promptTerminators), so child-process
// output (which is line-oriented in practice) is masked synchronously with
// the write that produced it while prompts stay visible.
type MaskingWriter struct {
	dst io.Writer

	mu   sync.Mutex
	tail string // unterminated partial line carried over from previous writes
}

// NewMaskingWriter returns a writer that forwards to dst with account IDs
// masked per line.
func NewMaskingWriter(dst io.Writer) *MaskingWriter {
	return &MaskingWriter{dst: dst}
}

func (mw *MaskingWriter) Write(p []byte) (int, error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	n := len(p)
	s := mw.tail + string(p)

	for {
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		if _, err := io.WriteString(mw.dst, MaskAccountIDs(s[:idx+1])); err != nil {
			return n, err
		}
		s = s[idx+1:]
	}
	// Forward an interactive question at once instead of withholding it
	// while the child waits for input; masking is unaffected because no
	// maskable pattern can span this split.
	if promptTerminated(s) {
		if _, err := io.WriteString(mw.dst, MaskAccountIDs(s)); err != nil {
			return n, err
		}
		s = ""
	}
	mw.tail = s
	return n, nil
}

// Flush masks and forwards any buffered partial line.
func (mw *MaskingWriter) Flush() {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	if mw.tail == "" {
		return
	}
	_, _ = io.WriteString(mw.dst, MaskAccountIDs(mw.tail))
	mw.tail = ""
}
