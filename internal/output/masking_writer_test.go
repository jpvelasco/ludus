package output

import (
	"bytes"
	"strings"
	"testing"
)

// TestMaskingWriterMasksAcrossWrites pins the #554 contract: account IDs are
// masked even when a line arrives split across several Write calls (as with
// child-process stderr), and the forwarded text is otherwise byte-identical.
func TestMaskingWriterMasksAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	mw := NewMaskingWriter(&buf)

	_, _ = mw.Write([]byte("The push refers to repository [123456789012"))
	_, _ = mw.Write([]byte(".dkr.ecr.us-east-1.amazonaws.com/ludus-server]\n"))

	got := buf.String()
	if strings.Contains(got, "123456789012") {
		t.Errorf("account ID not masked across split writes: %q", got)
	}
	if !strings.Contains(got, "************.dkr.ecr.us-east-1.amazonaws.com/ludus-server]\n") {
		t.Errorf("unexpected masked output: %q", got)
	}
}

func TestMaskingWriterBuffersPartialLine(t *testing.T) {
	var buf bytes.Buffer
	mw := NewMaskingWriter(&buf)

	_, _ = mw.Write([]byte("pushing 123456789012.dkr.ecr.us-east-1.amazonaws.com/x"))
	if buf.Len() != 0 {
		t.Errorf("partial line leaked before newline: %q", buf.String())
	}

	mw.Flush()
	if strings.Contains(buf.String(), "123456789012") {
		t.Errorf("flushed partial line not masked: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "************.dkr.ecr.us-east-1.amazonaws.com/x") {
		t.Errorf("unexpected flushed output: %q", buf.String())
	}
}
