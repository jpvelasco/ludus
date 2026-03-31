//go:build windows

package prereq

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (c *Checker) checkWindowsSDK() (CheckResult, int) {
	includeDir := filepath.Join(
		os.Getenv("ProgramFiles(x86)"),
		"Windows Kits", "10", "Include",
	)

	name, build, err := findHighestSDKBuild(includeDir)
	if err != nil {
		return CheckResult{
			Name:    "Windows SDK",
			Passed:  false,
			Message: err.Error(),
		}, 0
	}

	if build >= 26100 {
		return CheckResult{
			Name:    "Windows SDK",
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("SDK %s (build >= 26100 requires NNERuntimeORT patch)", name),
		}, build
	}

	return CheckResult{
		Name:    "Windows SDK",
		Passed:  true,
		Message: fmt.Sprintf("SDK %s", name),
	}, build
}

// findHighestSDKBuild scans includeDir for Windows SDK version directories
// and returns the one with the highest build number.
func findHighestSDKBuild(includeDir string) (string, int, error) {
	entries, err := os.ReadDir(includeDir)
	if err != nil {
		return "", 0, fmt.Errorf("cannot read %s: %v", includeDir, err)
	}

	type sdkVer struct {
		name  string
		build int
	}
	var versions []sdkVer
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		parts := strings.Split(e.Name(), ".")
		if len(parts) < 3 {
			continue
		}
		build, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		versions = append(versions, sdkVer{name: e.Name(), build: build})
	}

	if len(versions) == 0 {
		return "", 0, fmt.Errorf("no Windows SDK versions found in %s", includeDir)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].build > versions[j].build
	})

	return versions[0].name, versions[0].build, nil
}

// checkSmartAppControl detects whether Windows Smart App Control is active.
// Smart App Control blocks unsigned executables and DLLs, including all UE5
// binaries compiled from source. This causes cook failures with GetLastError=4551
// ("An Application Control policy has blocked this file"). The check reads the
// Code Integrity policy from the registry and scans the event log for recent blocks.
func (c *Checker) checkSmartAppControl() CheckResult {
	// Read Smart App Control state from registry.
	// VerifiedAndReputablePolicyState: 0=Off, 1=Enforce, 2=Evaluation
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`try { (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy' -ErrorAction Stop).VerifiedAndReputablePolicyState } catch { 'missing' }`).Output()
	if err != nil {
		return CheckResult{
			Name:    "Smart App Control",
			Passed:  true,
			Warning: true,
			Message: "could not read Code Integrity policy from registry",
		}
	}

	state := strings.TrimSpace(string(out))

	if state == "0" || state == "missing" {
		return CheckResult{
			Name:    "Smart App Control",
			Passed:  true,
			Message: "Smart App Control is off",
		}
	}

	mode := "enforcement"
	if state == "2" {
		mode = "evaluation"
	}

	blocked := c.scanCodeIntegrityBlocks()

	return CheckResult{
		Name:    "Smart App Control",
		Passed:  false,
		Message: buildSACMessage(mode, blocked),
	}
}

// buildSACMessage constructs the user-facing message for an active SAC detection.
func buildSACMessage(mode string, blocked []string) string {
	msg := fmt.Sprintf("Smart App Control (SAC) is in %s mode and will block unsigned DLLs compiled from source.\n", mode)
	msg += "  UE5 binaries built from source are unsigned and will be blocked, causing cook/build failures\n"
	msg += "  (GetLastError=4551). This also affects other developer tools like golangci-lint and clang.\n"
	msg += "  SAC is designed for end users, not developers who compile code from source.\n"
	msg += "  \n"
	msg += "  To fix: Turn off SAC\n"
	msg += "    Windows Security > App & browser control > Smart App Control > Off\n"
	msg += "  \n"
	msg += "  Important:\n"
	msg += "    - This does NOT disable Windows Defender antivirus. Real-time malware protection stays fully active.\n"
	msg += "    - This is irreversible without a Windows reinstall/reset (by Microsoft's design).\n"
	msg += "    - WDAC supplemental policies do NOT work with SAC (SAC's base policy is signed by\n"
	msg += "      Microsoft and rejects unsigned supplemental policies).\n"
	msg += "  \n"
	msg += "  Microsoft documentation:\n"
	msg += "    https://support.microsoft.com/en-us/topic/what-is-smart-app-control-285ea03d-fa88-4d56-882e-6698afdb7003\n"
	msg += "    https://learn.microsoft.com/en-us/windows/security/application-security/application-control/app-control-for-business/appcontrol"
	if len(blocked) > 0 {
		msg += fmt.Sprintf("\n  Recently blocked files (%d):", len(blocked))
		limit := len(blocked)
		if limit > 5 {
			limit = 5
		}
		for _, b := range blocked[:limit] {
			msg += "\n    - " + b
		}
		if len(blocked) > 5 {
			msg += fmt.Sprintf("\n    ... and %d more", len(blocked)-5)
		}
	}
	return msg
}

// scanCodeIntegrityBlocks queries the Windows Code Integrity event log for
// recent DLL blocks (Event ID 3077) affecting UE engine paths. Returns the
// list of blocked file paths (deduplicated).
func (c *Checker) scanCodeIntegrityBlocks() []string {
	// Query recent Code Integrity block events (ID 3077 = enforcement block)
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-WinEvent -FilterHashtable @{LogName='Microsoft-Windows-CodeIntegrity/Operational'; Id=3077} -MaxEvents 50 2>$null | `+
			`ForEach-Object { ($_.Message -split 'attempted to load ')[1] -split ' that did not meet' | Select-Object -First 1 }`).Output()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var blocked []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Convert device path to a friendlier name
		name := line
		if idx := strings.Index(name, `\Source Code\`); idx >= 0 {
			name = name[idx+1:]
		} else if idx := strings.LastIndex(name, `\`); idx >= 0 {
			name = name[idx+1:]
		}
		if !seen[name] {
			seen[name] = true
			blocked = append(blocked, name)
		}
	}
	return blocked
}
