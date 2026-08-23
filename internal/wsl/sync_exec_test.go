package wsl

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestSyncEngineSuccess(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})
	r := newTestRunner(t, false)

	res, err := SyncEngine(context.Background(), r, "Ubuntu", SyncOptions{
		SourcePath: `C:\ue5`,
		Version:    "5.7",
	})
	if err != nil {
		t.Fatalf("SyncEngine() error = %v", err)
	}
	if res.WSLPath != "$HOME/ludus/engine/5.7" {
		t.Errorf("WSLPath = %q, want $HOME/ludus/engine/5.7", res.WSLPath)
	}
	if res.DDCPath != NativeDDCDir {
		t.Errorf("DDCPath = %q, want %q", res.DDCPath, NativeDDCDir)
	}
}

func TestSyncEngineDefaultTargetDir(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})
	r := newTestRunner(t, false)

	res, err := SyncEngine(context.Background(), r, "Ubuntu", SyncOptions{
		SourcePath: `C:\ue5`,
		Version:    "",
	})
	if err != nil {
		t.Fatalf("SyncEngine() error = %v", err)
	}
	if res.WSLPath != "$HOME/ludus/engine/default" {
		t.Errorf("WSLPath = %q, want $HOME/ludus/engine/default", res.WSLPath)
	}
}

func TestSyncEngineExplicitTargetDir(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})
	r := newTestRunner(t, false)

	res, err := SyncEngine(context.Background(), r, "Ubuntu", SyncOptions{
		SourcePath: `C:\ue5`,
		Version:    "5.7",
		TargetDir:  "/custom/target",
	})
	if err != nil {
		t.Fatalf("SyncEngine() error = %v", err)
	}
	if res.WSLPath != "/custom/target" {
		t.Errorf("WSLPath = %q, want /custom/target", res.WSLPath)
	}
}

func TestSyncEngineInsufficientDisk(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  10G"})
	r := newTestRunner(t, false)

	_, err := SyncEngine(context.Background(), r, "Ubuntu", SyncOptions{SourcePath: `C:\ue5`})
	if err == nil || !strings.Contains(err.Error(), "insufficient disk space") {
		t.Errorf("SyncEngine() error = %v, want 'insufficient disk space'", err)
	}
}

func TestSyncEngineDiskCheckError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	r := newTestRunner(t, false)

	_, err := SyncEngine(context.Background(), r, "Ubuntu", SyncOptions{SourcePath: `C:\ue5`})
	if err == nil || !strings.Contains(err.Error(), "checking disk space") {
		t.Errorf("SyncEngine() error = %v, want 'checking disk space'", err)
	}
}

func TestSyncEngineEmptySource(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})
	r := newTestRunner(t, false)

	_, err := SyncEngine(context.Background(), r, "Ubuntu", SyncOptions{SourcePath: ""})
	if err == nil || !strings.Contains(err.Error(), "empty source path") {
		t.Errorf("SyncEngine() error = %v, want 'empty source path'", err)
	}
}

// TestSyncEngineQuotesSpecialCharacters pins the #566 contract: engine paths
// containing apostrophes or spaces must reach the shell safely — the rsync
// source single-quoted with embedded quotes escaped, the mkdir/rsync targets
// double-quoted (preserving $HOME expansion) instead of bare.
func TestSyncEngineQuotesSpecialCharacters(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})

	var echo bytes.Buffer
	r := runner.NewRunner(true, false)
	r.Stdout = &echo
	r.Stderr = io.Discard

	res, err := SyncEngine(context.Background(), r, "Ubuntu", SyncOptions{
		SourcePath: `D:\Data\Pat's Drive\ue5`,
		Version:    "5.7",
	})
	if err != nil {
		t.Fatalf("SyncEngine() error = %v", err)
	}
	if res.WSLPath != "$HOME/ludus/engine/5.7" {
		t.Errorf("WSLPath = %q, want $HOME/ludus/engine/5.7", res.WSLPath)
	}

	logged := echo.String()
	const wantSource = `'/mnt/d/Data/Pat'\''s Drive/ue5/'`
	if !strings.Contains(logged, wantSource) {
		t.Errorf("rsync source not safely quoted; want %s in:\n%s", wantSource, logged)
	}
	if !strings.Contains(logged, `mkdir -p "$HOME/ludus/engine/5.7"`) {
		t.Errorf("mkdir target not double-quoted:\n%s", logged)
	}
}
