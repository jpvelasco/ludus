package output

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// TestMaskingWriterEquivalenceProperty is the strong form of the mask-boundary
// invariant for #709's early forwarding: for any chunking of a stream, the
// concatenated output forwarded by the masking writer (complete lines,
// prompt-terminated partials, and the final Flush) must be byte-identical to
// masking the whole stream in a single pass. This holds because forwarding
// only ever splits at a newline or a prompt terminator (both impossible inside
// a maskable account-ID pattern), so a pattern can never be split across an
// unmasked boundary.
func TestMaskingWriterEquivalenceProperty(t *testing.T) {
	pieces := []string{
		"arn:aws:gamelift:us-east-1:", "123456789012", ":fleet/x\n",
		"push ", "999123456789012.dkr.ecr", ".us-east-1.amazonaws.com/x\n",
		"Region [us-east-1]: ", "x", "\n", "? ", "id: ",
		"123456789012.dkr.ecr.us-east-1.amazonaws.com\n",
		"arn:aws:s3:us-east-1:123456789012:bucket\n",
		"plain text\n", "999", ":", " ", "a", "arn", "arn:aws:x:",
		"123456789012.d", "kr.ecr.us-east-1.amazonaws.com/x\n",
		"arn:aws:gamelift:us-east-1:123456789012", ":fleet/y\n",
	}
	// #nosec G404 -- fixed-seed PRNG generates deterministic test input, not security values //nolint:gosec
	rng := rand.New(rand.NewSource(1))
	for iter := 0; iter < 5000; iter++ {
		var full strings.Builder
		var chunks []string
		n := 1 + rng.Intn(30)
		for i := 0; i < n; i++ {
			p := pieces[rng.Intn(len(pieces))]
			full.WriteString(p)
			chunks = append(chunks, p)
		}

		var buf bytes.Buffer
		mw := NewMaskingWriter(&buf)
		for _, chunk := range chunks {
			if _, err := mw.Write([]byte(chunk)); err != nil {
				t.Fatalf("iter %d: Write: %v", iter, err)
			}
		}
		mw.Flush()

		want := MaskAccountIDs(full.String())
		if buf.String() != want {
			t.Fatalf("iter %d: chunked masking diverged from single-pass masking\nfull=%q\nwant=%q\ngot =%q", iter, full.String(), want, buf.String())
		}
	}
}
