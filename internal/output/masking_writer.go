package output

import (
	"io"
	"strings"
	"sync"
)

// MaskingWriter wraps an io.Writer and masks AWS account IDs in every
// complete line before forwarding it. Partial lines are buffered until a
// newline arrives or Flush is called, so child-process output (which is
// line-oriented in practice) is masked synchronously with the write that
// produced it.
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
