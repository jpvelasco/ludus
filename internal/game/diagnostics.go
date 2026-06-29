package game

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// diagnoseBuildError inspects a build error for known failure patterns and
// returns an error with actionable guidance. Scans the RunUAT log file for
// common error patterns and appends diagnostics to the error message.
func diagnoseBuildError(err error, action, enginePath string) error {
	if err == nil {
		return nil
	}
	if isSmartScreenExit(err) {
		return smartScreenError(err, action)
	}
	return buildLogError(err, action, enginePath)
}

func isSmartScreenExit(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.ExitCode() == 0xC0E90002
}

func smartScreenError(err error, action string) error {
	return fmt.Errorf("%s failed (exit code 0xC0E90002): Windows SmartScreen/UAC "+
		"may be blocking a freshly-built executable. Try one of:\n"+
		"  1. Run 'ludus game build' as Administrator (one time only)\n"+
		"  2. Navigate to Engine/Binaries/Win64/ and run UnrealEditor-Cmd.exe manually to approve it\n"+
		"  3. Right-click the blocked .exe -> Properties -> Unblock\n"+
		"Original error: %w", action, err)
}

func buildLogError(err error, action, enginePath string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s failed", action)
	appendBuildLogHints(&b, enginePath)
	fmt.Fprintf(&b, "\n\nFull build log: %s", buildLogPath(enginePath))
	return fmt.Errorf("%s: %w", b.String(), err)
}

func appendBuildLogHints(b *strings.Builder, enginePath string) {
	hints := scanBuildLogs(enginePath)
	if len(hints) == 0 {
		return
	}
	b.WriteString("\n\nDiagnostics from build logs:")
	for _, h := range hints {
		b.WriteString("\n  - ")
		b.WriteString(h)
	}
}

func buildLogPath(enginePath string) string {
	return filepath.Join(enginePath, "Engine", "Programs", "AutomationTool", "Saved", "Logs", "Log.txt")
}

// knownLogPattern maps a string found in build logs to user-facing guidance.
type knownLogPattern struct {
	pattern string
	hint    string
}

// knownLogPatterns is the table of error patterns discovered during E2E testing.
// Each pattern is a substring to search for in the RunUAT log file, paired with
// actionable guidance for the user.
var knownLogPatterns = []knownLogPattern{
	{"GameFeatureData is missing",
		"Missing game content — run 'ludus init --fix' to overlay content from Epic Launcher"},
	{"C1076: compiler limit",
		"Out of memory during compilation — reduce parallel jobs with -j flag (e.g. -j 4)"},
	{"C3859: Failed to create virtual memory",
		"Out of memory during PCH compilation — reduce parallel jobs with -j flag"},
	{"GetLastError=4551",
		"Windows Smart App Control (SAC) blocked a DLL. SAC blocks all unsigned binaries, including UE5 code compiled from source. " +
			"Turn off SAC (this does NOT disable Windows Defender antivirus): " +
			"Windows Security > App & browser control > Smart App Control > Off. " +
			"Run 'ludus init' for details. " +
			"See: https://support.microsoft.com/en-us/topic/what-is-smart-app-control-285ea03d-fa88-4d56-882e-6698afdb7003"},
	{"NU1903",
		"NuGet security audit failure — this should be handled automatically; please report as a bug"},
	{"error C4756:",
		"Overflow in constant arithmetic (MSVC C4756) — run 'ludus init --fix' to patch affected source files"},
	{"code integrity policy",
		"Windows Smart App Control (SAC) blocked execution — run 'ludus init' to see Microsoft's recommended options"},
	{"LINUX_MULTIARCH_ROOT",
		"Linux cross-compile toolchain not found — run 'ludus init --fix' to install, then restart your terminal"},
	{"AddBuildProductsFromManifest",
		"A build product listed in the UBT manifest was not generated. " +
			"For .sym files on ARM64 cross-compile, this is caused by dump_syms.exe crashing — " +
			"ludus should handle this automatically; please report as a bug if it persists"},
	{"has build products in common with",
		"Build-settings mismatch: a project target pins an older DefaultBuildSettings " +
			"(e.g. BuildSettingsVersion.V6) whose warning levels conflict with this engine's " +
			"defaults (UE 5.8 promotes Unreachable/ReturnType/Dangling warnings to errors). " +
			"Bump DefaultBuildSettings to BuildSettingsVersion.Latest (or this engine's version) " +
			"in your *.Target.cs files"},
}

// scanBuildLogs reads the RunUAT log file and returns hints for any known
// error patterns found. Returns nil if the log doesn't exist or no patterns match.
func scanBuildLogs(enginePath string) []string {
	if enginePath == "" {
		return nil
	}

	data, err := os.ReadFile(buildLogPath(enginePath))
	if err != nil {
		return nil
	}

	return matchBuildLogHints(string(data))
}

func matchBuildLogHints(content string) []string {
	var hints []string
	seen := make(map[string]bool)
	for _, p := range knownLogPatterns {
		if strings.Contains(content, p.pattern) && !seen[p.hint] {
			hints = append(hints, p.hint)
			seen[p.hint] = true
		}
	}
	return hints
}
