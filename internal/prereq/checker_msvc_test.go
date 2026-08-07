//go:build windows

package prereq

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNeedsNewerMSVC pins the engine-version gate that selects the MSVC toolset.
// UE 5.7+ (incl. 5.8) needs MSVC 14.44; 5.6 and earlier use 14.38. A drift in
// the `minor >= 7` gate would silently mis-pin MSVC for 5.8 and break Windows
// container builds, so 5.8 is asserted explicitly here. Passing only a config
// version exercises DetectEngineVersion's config fallback (no engine tree needed).
func TestNeedsNewerMSVC(t *testing.T) {
	tests := []struct {
		configVersion string
		want          bool
	}{
		{"5.4.4", false},
		{"5.5.4", false},
		{"5.6.1", false},
		{"5.7.3", true},
		{"5.8.0", true},
		{"", false},
		{"garbage", false},
		{"5.abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.configVersion, func(t *testing.T) {
			if got := needsNewerMSVC("", tt.configVersion); got != tt.want {
				t.Errorf("needsNewerMSVC(%q) = %v, want %v", tt.configVersion, got, tt.want)
			}
		})
	}
}

// TestMSVCVersionForEngine confirms the gate maps to the right MSVC toolset
// string — the value actually written into BuildConfiguration.xml.
func TestMSVCVersionForEngine(t *testing.T) {
	tests := []struct {
		configVersion string
		want          string
	}{
		{"5.6.1", "14.38.33130"},
		{"5.7.3", "14.44.35207"},
		{"5.8.0", "14.44.35207"},
	}
	for _, tt := range tests {
		t.Run(tt.configVersion, func(t *testing.T) {
			if got := msvcVersionForEngine("", tt.configVersion); got != tt.want {
				t.Errorf("msvcVersionForEngine(%q) = %q, want %q", tt.configVersion, got, tt.want)
			}
		})
	}
}

// containsProductsStar reports whether args scope vswhere to all products,
// which is what makes the BuildTools edition discoverable.
func containsProductsStar(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-products" && args[i+1] == "*" {
			return true
		}
	}
	return false
}

// TestVswhereArgsScopeAllProducts guards the #411 fix: both the instance-listing
// and component-checking vswhere invocations must pass "-products *", otherwise
// a headless VS 2022 Build Tools install is reported as "no Visual Studio detected".
func TestVswhereArgsScopeAllProducts(t *testing.T) {
	if !containsProductsStar(vswhereListArgs()) {
		t.Errorf("vswhereListArgs() = %v; missing -products *", vswhereListArgs())
	}
	reqArgs := vswhereRequiresArgs("Microsoft.VisualStudio.Component.VC.Tools.x86.x64")
	if !containsProductsStar(reqArgs) {
		t.Errorf("vswhereRequiresArgs() = %v; missing -products *", reqArgs)
	}
}

// TestVswhereRequiresArgsIncludesComponent confirms the component id is threaded
// into the -requires query.
func TestVswhereRequiresArgsIncludesComponent(t *testing.T) {
	const id = "Microsoft.VisualStudio.Component.VC.14.44.17.14.x86.x64"
	args := vswhereRequiresArgs(id)
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-requires" && args[i+1] == id {
			found = true
		}
	}
	if !found {
		t.Errorf("vswhereRequiresArgs(%q) = %v; component id not present", id, args)
	}
}

// writeBatStub writes a minimal .bat that echoes stdout and exits with code.
// Used to impersonate vswhere.exe/setup.exe at arbitrary absolute paths.
func writeBatStub(t *testing.T, dir, name string, exitCode int, stdout string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	body := "@echo off\r\n"
	if stdout != "" {
		body += "echo " + stdout + "\r\n"
	}
	if exitCode != 0 {
		body += fmt.Sprintf("exit /b %d\r\n", exitCode)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindVSInstallsBranches(t *testing.T) {
	const validJSON = `[{"displayName":"Visual Studio Community 2022","installationPath":"C:\\VS2022"}]`
	tests := []struct {
		name     string
		setup    func(*testing.T) string
		wantErr  string
		wantInst bool
	}{
		{
			name: "vswhere missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.exe")
			},
			wantErr: "vswhere.exe not found",
		},
		{
			name: "vswhere fails",
			setup: func(t *testing.T) string {
				return writeBatStub(t, t.TempDir(), "vswhere.bat", 1, "")
			},
			wantErr: "vswhere failed",
		},
		{
			name: "no installations",
			setup: func(t *testing.T) string {
				return writeBatStub(t, t.TempDir(), "vswhere.bat", 0, "[]")
			},
			wantErr: "no Visual Studio installation",
		},
		{
			name: "malformed json",
			setup: func(t *testing.T) string {
				return writeBatStub(t, t.TempDir(), "vswhere.bat", 0, "garbage")
			},
			wantErr: "no Visual Studio installation",
		},
		{
			name: "valid installation",
			setup: func(t *testing.T) string {
				return writeBatStub(t, t.TempDir(), "vswhere.bat", 0, validJSON)
			},
			wantInst: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installs, err := findVSInstalls(tt.setup(t))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("findVSInstalls() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || len(installs) != 1 || installs[0].DisplayName == "" {
				t.Errorf("findVSInstalls() = (%+v, %v), want one install", installs, err)
			}
		})
	}
}

func TestFindMissingComponentsBranches(t *testing.T) {
	const validJSON = `[{"displayName":"Visual Studio Community 2022","installationPath":"C:\\VS2022"}]`
	required := []vsComponent{
		{"Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "C++ build tools"},
		{"Microsoft.VisualStudio.Component.VC.14.38.17.8.x86.x64", "MSVC v14.38"},
	}
	tests := []struct {
		name     string
		stdout   string
		exitCode int
		want     int
	}{
		{"tool exits non-zero", "", 1, 2},
		{"tool returns no instances", "[]", 0, 2},
		{"tool returns instances", validJSON, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeBatStub(t, t.TempDir(), "vswhere.bat", tt.exitCode, tt.stdout)
			got := findMissingComponents(path, required)
			if len(got) != tt.want {
				t.Errorf("findMissingComponents() = %v, want %d missing", got, tt.want)
			}
		})
	}
}

// writeSetupExe creates ProgramFiles(x86)/Microsoft Visual Studio/Installer/
// setup.exe under root and returns the installer directory path.
func writeSetupExe(t *testing.T, root string) string {
	t.Helper()
	installerDir := filepath.Join(root, "Microsoft Visual Studio", "Installer")
	if err := os.MkdirAll(installerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installerDir, "setup.exe"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	return installerDir
}

func fixVSComponentArgs() (vswhereResult, []string, []vsComponent) {
	return vswhereResult{InstallationPath: `C:\VS2022`},
		[]string{"C++ build tools"},
		[]vsComponent{{"X", "C++ build tools"}}
}

func TestFixMissingVSComponents_SetupExeMissing(t *testing.T) {
	t.Setenv("ProgramFiles(x86)", t.TempDir())
	install, missing, required := fixVSComponentArgs()
	got := (&Checker{}).fixMissingVSComponents(install, missing, required)
	if got.Passed || !strings.Contains(got.Message, "setup.exe not found") {
		t.Errorf("fixMissingVSComponents() = %+v, want setup.exe not found failure", got)
	}
}

func TestFixMissingVSComponents_StartFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramFiles(x86)", root)
	writeSetupExe(t, root)
	t.Setenv("PATH", t.TempDir())
	install, missing, required := fixVSComponentArgs()
	got := (&Checker{}).fixMissingVSComponents(install, missing, required)
	if got.Passed || !strings.Contains(got.Message, "failed to launch") {
		t.Errorf("fixMissingVSComponents() = %+v, want launch failure", got)
	}
}

func TestFixMissingVSComponents_Launches(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramFiles(x86)", root)
	writeSetupExe(t, root)
	sacBatchFake(t, "ok", 0)
	install, missing, required := fixVSComponentArgs()
	got := (&Checker{}).fixMissingVSComponents(install, missing, required)
	if !got.Passed || !got.Warning || !strings.Contains(got.Message, "launched VS Installer") {
		t.Errorf("fixMissingVSComponents() = %+v, want launched warning", got)
	}
}
