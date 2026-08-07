//go:build windows

package game

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestDiagnoseBuildError_SmartScreenExit(t *testing.T) {
	// powershell exit codes wrap mod 2^32, so `exit -1058471934` yields the
	// magic 0xC0E90002 DWORD — the SmartScreen/UAC exit code.
	runErr := exec.Command("powershell", "-NoProfile", "-Command", "exit -1058471934").Run()
	if runErr == nil {
		t.Fatal("expected non-zero exit from powershell")
	}
	if !isSmartScreenExit(runErr) {
		t.Errorf("isSmartScreenExit() = false for exit code 0xC0E90002")
	}
	got := diagnoseBuildError(runErr, "game build", t.TempDir())
	if !strings.Contains(got.Error(), "SmartScreen") || !strings.Contains(got.Error(), "game build") {
		t.Errorf("diagnoseBuildError() = %v, want SmartScreen guidance", got)
	}
}

func TestEnsureLinuxMultiarchRoot_RegistryFallback(t *testing.T) {
	t.Run("registry value appended", func(t *testing.T) {
		t.Setenv("LINUX_MULTIARCH_ROOT", "")
		testsupport.FakeTool(t, "reg", testsupport.ToolBehavior{
			Stdout: "    LINUX_MULTIARCH_ROOT    REG_SZ    C:\\toolchains\\v26_clang-20",
		})
		b := NewBuilder(BuildOptions{}, runner.NewRunner(false, true))
		b.ensureLinuxMultiarchRoot()
		if len(b.Runner.Env) != 1 || !strings.Contains(b.Runner.Env[0], "v26_clang-20") {
			t.Errorf("runner env = %v, want registry fallback appended", b.Runner.Env)
		}
	})

	t.Run("registry unreadable appends nothing", func(t *testing.T) {
		t.Setenv("LINUX_MULTIARCH_ROOT", "")
		testsupport.FakeTool(t, "reg", testsupport.ToolBehavior{ExitCode: 1})
		b := NewBuilder(BuildOptions{}, runner.NewRunner(false, true))
		b.ensureLinuxMultiarchRoot()
		if len(b.Runner.Env) != 0 {
			t.Errorf("runner env = %v, want empty", b.Runner.Env)
		}
	})
}
