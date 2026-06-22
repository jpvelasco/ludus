package output

import (
	"bufio"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Redirect installs a filter over os.Stdout that masks AWS account IDs in
// every line written through it. It works by pointing os.Stdout at the write
// end of an os.Pipe and draining the read end through MaskAccountIDs in a
// background goroutine. Use Install to set it up and Close to restore and drain.
type Redirect struct {
	real  *os.File // the original os.Stdout, restored on Close
	w     *os.File // pipe write end (becomes os.Stdout)
	wg    sync.WaitGroup
	once  sync.Once
	sigCh chan os.Signal
}

// Install redirects os.Stdout through the account-ID masking filter and returns
// a *Redirect whose Close restores the original stdout. The caller is
// responsible for calling Close (e.g. via defer) so buffered output is flushed.
func Install() (*Redirect, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	red := &Redirect{
		real:  os.Stdout,
		w:     w,
		sigCh: make(chan os.Signal, 1),
	}
	os.Stdout = w

	red.wg.Add(1)
	go red.drain(r)

	// Ensure buffered output is flushed if the process is interrupted: Cobra
	// installs no handler that would run our deferred Close, so a raw signal
	// would otherwise lose in-flight lines.
	signal.Notify(red.sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		if _, ok := <-red.sigCh; ok {
			red.Close()
		}
	}()

	return red, nil
}

// drain reads the pipe line by line, masks each chunk, and writes it to the
// real stdout. It uses ReadString rather than bufio.Scanner so that lines
// longer than 64 KB are not silently truncated, and it flushes the final
// non-newline-terminated chunk at EOF.
func (red *Redirect) drain(r *os.File) {
	defer red.wg.Done()
	defer func() { _ = r.Close() }()

	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			_, _ = io.WriteString(red.real, MaskAccountIDs(line))
		}
		if err != nil {
			return
		}
	}
}

// Close restores os.Stdout, stops the signal handler, and waits for all
// buffered output to be masked and flushed. It is safe to call multiple times
// and on a nil receiver.
func (red *Redirect) Close() {
	if red == nil {
		return
	}
	red.once.Do(func() {
		signal.Stop(red.sigCh)
		close(red.sigCh)
		os.Stdout = red.real
		_ = red.w.Close() // signals EOF to drain
		red.wg.Wait()
	})
}
