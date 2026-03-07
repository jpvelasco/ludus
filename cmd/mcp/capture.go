package mcp

import (
	"bytes"
	"io"
	"os"
	"sync"
)

// capturedOutput holds stdout and stderr captured during a function call.
type capturedOutput struct {
	Stdout string
	Stderr string
}

// withCapture redirects os.Stdout and os.Stderr to pipes during fn execution,
// capturing all output. This is safe because MCP stdio processes requests
// sequentially. The original file descriptors are restored after fn returns.
func withCapture(fn func() error) (capturedOutput, error) {
	origStdout := os.Stdout
	origStderr := os.Stderr

	outR, outW, err := os.Pipe()
	if err != nil {
		return capturedOutput{}, fn()
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		return capturedOutput{}, fn()
	}

	os.Stdout = outW
	os.Stderr = errW

	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(&outBuf, outR)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&errBuf, errR)
	}()

	fnErr := fn()

	// Close write ends so readers finish
	_ = outW.Close()
	_ = errW.Close()
	wg.Wait()
	_ = outR.Close()
	_ = errR.Close()

	// Restore original file descriptors
	os.Stdout = origStdout
	os.Stderr = origStderr

	return capturedOutput{
		Stdout: outBuf.String(),
		Stderr: errBuf.String(),
	}, fnErr
}
