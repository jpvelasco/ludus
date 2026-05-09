package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devrecon/ludus/internal/toolchain"
)

// promptEnginePath scans for engine directories and lets the user pick or type a path.
func promptEnginePath() string {
	candidates := scanEnginePaths()

	if len(candidates) == 0 {
		return prompt("Engine source path (or press Enter to skip)", "")
	}

	printEngineCandidates(candidates)
	answer := readEngineCandidateChoice()
	for i, c := range candidates {
		if answer == fmt.Sprintf("%d", i+1) {
			return c
		}
	}
	if answer == fmt.Sprintf("%d", len(candidates)+1) {
		return prompt("Engine source path", "")
	}
	return ""
}

func printEngineCandidates(candidates []string) {
	fmt.Println("Found engine source directories:")
	for i, c := range candidates {
		fmt.Printf("  %d) %s%s\n", i+1, c, engineVersionLabel(c))
	}
	fmt.Printf("  %d) Enter a different path\n", len(candidates)+1)
	fmt.Printf("  %d) Skip (configure later)\n", len(candidates)+2)
}

func engineVersionLabel(path string) string {
	bv, err := toolchain.ParseBuildVersion(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(" (v%d.%d.%d)", bv.MajorVersion, bv.MinorVersion, bv.PatchVersion)
}

func readEngineCandidateChoice() string {
	fmt.Printf("Choice [1]: ")
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return "1"
	}
	return answer
}

// scanEnginePaths looks for Unreal Engine source directories in common locations.
func scanEnginePaths() []string {
	var candidates []string
	seen := make(map[string]bool)

	addIfEngine := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil || seen[abs] || !isEngineSourceDir(abs) {
			return
		}
		candidates = append(candidates, abs)
		seen[abs] = true
	}

	scanWorkingTreeEnginePaths(addIfEngine)
	scanHomeEnginePaths(addIfEngine)
	scanPlatformEnginePaths(addIfEngine)

	return candidates
}

func isEngineSourceDir(path string) bool {
	if _, err := os.Stat(filepath.Join(path, engineSetupFile())); err != nil {
		return false
	}
	return true
}

func engineSetupFile() string {
	if runtime.GOOS == "windows" {
		return "Setup.bat"
	}
	return "Setup.sh"
}

func scanWorkingTreeEnginePaths(fn func(string)) {
	cwd, _ := os.Getwd()
	if cwd == "" {
		return
	}
	scanGlob(filepath.Dir(cwd), "UnrealEngine*", fn)
}

func scanHomeEnginePaths(fn func(string)) {
	home, _ := os.UserHomeDir()
	if home == "" {
		return
	}
	scanGlob(filepath.Join(home, "Documents", "Source"), "UnrealEngine*", fn)
	scanGlob(filepath.Join(home, "Source"), "UnrealEngine*", fn)
}

func scanPlatformEnginePaths(fn func(string)) {
	if runtime.GOOS == "windows" {
		scanWindowsEnginePaths(fn)
		return
	}
	scanGlob("/opt", "UnrealEngine*", fn)
	scanGlob("/usr/local/src", "UnrealEngine*", fn)
}

func scanWindowsEnginePaths(fn func(string)) {
	for _, drive := range []string{"C:", "D:", "E:", "F:"} {
		root := filepath.Join(drive, string(os.PathSeparator))
		scanGlob(filepath.Join(root, "Source Code"), "UnrealEngine*", fn)
		scanGlob(filepath.Join(root, "Source"), "UnrealEngine*", fn)
		scanGlob(root, "UnrealEngine*", fn)
	}
}

// scanGlob searches for directories matching pattern inside dir and calls fn for each.
func scanGlob(dir, pattern string, fn func(string)) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return
	}
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}
		fn(m)
	}
}
