//go:build windows

package game

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestReadSystemEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		behavior testsupport.ToolBehavior
		want     string
	}{
		{
			name:     "reg query fails",
			behavior: testsupport.ToolBehavior{ExitCode: 1},
			want:     "",
		},
		{
			name:     "REG_SZ value",
			behavior: testsupport.ToolBehavior{Stdout: "    LINUX_MULTIARCH_ROOT    REG_SZ    C:\\UnrealToolchains\\v26_clang-20.1.8-rockylinux8"},
			want:     "C:\\UnrealToolchains\\v26_clang-20.1.8-rockylinux8",
		},
		{
			name:     "REG_EXPAND_SZ value",
			behavior: testsupport.ToolBehavior{Stdout: "    LINUX_MULTIARCH_ROOT    REG_EXPAND_SZ    %%NOTDEFINED_ANYWHERE%%\\tools"},
			want:     "%NOTDEFINED_ANYWHERE%\\tools",
		},
		{
			name:     "garbage output",
			behavior: testsupport.ToolBehavior{Stdout: "not reg output"},
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "reg", tt.behavior)
			got := readSystemEnvVar("LINUX_MULTIARCH_ROOT")
			if got != tt.want {
				t.Errorf("readSystemEnvVar() = %q, want %q", got, tt.want)
			}
		})
	}
}
