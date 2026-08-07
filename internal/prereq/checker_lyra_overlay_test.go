package prereq

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestCPOverlay_Success(t *testing.T) {
	testsupport.FakeTool(t, "cp", testsupport.ToolBehavior{ExitCode: 0})
	if err := (&Checker{}).cpOverlay("/src", "/dst"); err != nil {
		t.Errorf("cpOverlay() error = %v, want nil", err)
	}
}

func TestCPOverlay_Failure(t *testing.T) {
	testsupport.FakeTool(t, "cp", testsupport.ToolBehavior{ExitCode: 1})
	if err := (&Checker{}).cpOverlay("/src", "/dst"); err == nil {
		t.Error("cpOverlay() error = nil, want failure")
	}
}
