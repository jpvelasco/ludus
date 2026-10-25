package output

import (
	"bytes"
	"io"
	"os"
	"sync"
)

// Redirect installs a filter over os.Stdout that masks AWS account IDs in
// every line written through it. It works by pointing os.Stdout at the write
// end of an os.Pipe and draining the read end through MaskAccountIDs in a
// background goroutine. Use Install to set it up and Close to restore and drain.
type Redirect struct {
	real *os.File // the original os.Stdout, restored on Close
	w    *os.File // pipe write end (becomes os.Stdout)
	wg   sync.WaitGroup
	once sync.Once
}

// Install redirects os.Stdout through the account-ID masking filter and returns
// a *Redirect whose Close restores the original stdout. The caller is
// responsible for calling Close (e.g. via defer) so buffered output is flushed.
//
// Install registers no signal handler: termination signals belong to the
// caller's root context (the CLI subscribes to SIGINT and SIGTERM there and
// runs this Close on teardown), so the masking layer never consumes a
// termination signal by itself (#710).
func Install() (*Redirect, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	red := &Redirect{
		real: os.Stdout,
		w:    w,
	}
	os.Stdout = w

	red.wg.Add(1)
	go red.drain(r)

	return red, nil
}

// drain reads the pipe as chunks arrive, masks and forwards every complete
// line plus any prompt-terminated partial line (an interactive question the
// process is now waiting on — see promptTerminators), and flushes the final
// non-newline-terminated remainder at EOF. Raw chunk reads keep lines of any
// length intact while returning as soon as data arrives, so a prompt is not
// held hostage to a newline that never comes while the process waits for
// input (#709).
func (red *Redirect) drain(r *os.File) {
	defer red.wg.Done()
	defer func() { _ = r.Close() }()

	buf := make([]byte, 32*1024)
	var pending []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			pending = red.forward(pending)
		}
		if err != nil {
			// EOF (or a broken pipe): no more bytes can arrive, so the whole
			// remainder is masked and flushed without holding a boundary tail.
			red.write(MaskAccountIDs(string(pending)))
			return
		}
	}
}

// forward masks and writes every complete line of p plus any prompt-terminated
// remainder, returning the bytes that must stay buffered for now.
func (red *Redirect) forward(p []byte) []byte {
	for {
		idx := bytes.IndexByte(p, '\n')
		if idx < 0 {
			break
		}
		red.write(MaskAccountIDs(string(p[:idx+1])))
		p = p[idx+1:]
	}
	rest := string(p)
	if promptTerminated(rest) {
		red.write(MaskAccountIDs(rest))
		return nil
	}
	return p
}

// write forwards masked text to the process's real stdout.
func (red *Redirect) write(s string) {
	// #nosec G705 -- red.real is the process's real stdout (a terminal/file),
	// not a web response; XSS does not apply. The data is also masked here.
	_, _ = io.WriteString(red.real, s)
}

// Close restores os.Stdout and waits for all buffered output to be masked and
// flushed. It is safe to call multiple times and on a nil receiver.
func (red *Redirect) Close() {
	if red == nil {
		return
	}
	red.once.Do(func() {
		os.Stdout = red.real
		_ = red.w.Close() // signals EOF to drain
		red.wg.Wait()
	})
}
