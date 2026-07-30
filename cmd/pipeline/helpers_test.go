package pipeline

import (
	"bytes"
	"io"
	"os"
)

// captureStdout captures stdout during fn execution and returns the captured output.
func captureStdout(fn func()) string {
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		return ""
	}
	defer func() { _ = r.Close() }()

	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan bool, 1)
	go func() {
		_, _ = io.Copy(&buf, r)
		done <- true
	}()

	fn()

	_ = w.Close()
	<-done

	return buf.String()
}
