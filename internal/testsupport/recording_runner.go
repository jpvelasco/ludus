package testsupport

import (
	"bytes"
	"strings"

	"github.com/jpvelasco/ludus/internal/runner"
)

// RecordingRunner returns a *runner.Runner in dry-run mode whose echoed
// command lines are captured, plus a func to read them back.
func RecordingRunner() (*runner.Runner, func() []string) {
	buf := &bytes.Buffer{}
	r := runner.NewRunner(false, true)
	r.Stdout = buf

	reader := func() []string {
		var lines []string
		for line := range strings.SplitSeq(buf.String(), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed, ok := strings.CutPrefix(trimmed, "+"); ok {
				lines = append(lines, trimmed)
			}
		}
		return lines
	}

	return r, reader
}
