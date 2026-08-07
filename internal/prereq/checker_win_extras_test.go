//go:build windows

package prereq

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/toolchain"
)

// sacBatchFake writes a raw powershell.bat stub whose stdout is made of one
// echo line per line in content, then restricts PATH to that stub dir so the
// fake wins over the real powershell.exe in System32. Bare lines in a .bat are
// executed as commands by cmd.exe, so the stub must use explicit echo lines.
func sacBatchFake(t *testing.T, content string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	var sb strings.Builder
	sb.WriteString("@echo off\r\n")
	for _, line := range strings.Split(content, "\n") {
		if line != "" {
			sb.WriteString("echo " + line + "\r\n")
		}
	}
	if exitCode != 0 {
		fmt.Fprintf(&sb, "exit /b %d\r\n", exitCode)
	}
	if err := os.WriteFile(filepath.Join(dir, "powershell.bat"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestCheckSmartAppControl_Off(t *testing.T) {
	sacBatchFake(t, "0", 0)
	res := (&Checker{}).checkSmartAppControl()
	if !res.Passed || !strings.Contains(res.Message, "Smart App Control is off") {
		t.Fatalf("checkSmartAppControl() = %+v", res)
	}
}

func TestCheckSmartAppControl_ActiveWithBlocks(t *testing.T) {
	// Policy reports enforcement ("1") on the first powershell read; the block
	// scan sees the same output and turns the trailing line into a block entry.
	// The state parser switches on the full trimmed output, so the fake must
	// output exactly "1" for the enforcement path.
	sacBatchFake(t, "1", 0)
	res := (&Checker{}).checkSmartAppControl()
	if res.Passed {
		t.Fatalf("expected SAC failure when enforcing, got %+v", res)
	}
	if !strings.Contains(res.Message, "enforcement mode") {
		t.Fatalf("checkSmartAppControl() = %+v", res)
	}
}

func TestCheckSmartAppControl_PolicyUnreadable(t *testing.T) {
	sacBatchFake(t, "", 1)
	res := (&Checker{}).checkSmartAppControl()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "could not read Code Integrity policy") {
		t.Fatalf("checkSmartAppControl() = %+v", res)
	}
}

func TestScanCodeIntegrityBlocks_DedupesAndNames(t *testing.T) {
	sacBatchFake(t, "A.dll\n\nB.dll\nB.dll\n", 0)
	got := (&Checker{}).scanCodeIntegrityBlocks()
	if len(got) != 2 || got[0] != "A.dll" || got[1] != "B.dll" {
		t.Fatalf("scanCodeIntegrityBlocks() = %v", got)
	}
}

// TestScanCodeIntegrityBlocks_PowershellUnavailable covers the error branch:
// when the Code Integrity event log query fails, the scan degrades to nil.
func TestScanCodeIntegrityBlocks_PowershellUnavailable(t *testing.T) {
	sacBatchFake(t, "", 1)
	if got := (&Checker{}).scanCodeIntegrityBlocks(); got != nil {
		t.Fatalf("scanCodeIntegrityBlocks() = %v, want nil on powershell failure", got)
	}
}

// TestScanCodeIntegrityBlocks_SourceCodePath covers the device-path
// shortening branch: a blocked path under \Source Code\ is trimmed to the
// readable prefix instead of the bare file name.
func TestScanCodeIntegrityBlocks_SourceCodePath(t *testing.T) {
	sacBatchFake(t, `C:\Users\me\Source Code\UE5\Game\Foo.dll`, 0)
	got := (&Checker{}).scanCodeIntegrityBlocks()
	want := "Source Code\\UE5\\Game\\Foo.dll"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("scanCodeIntegrityBlocks() = %v, want [%q]", got, want)
	}
}

// sdkEngineFixture builds an engine tree and a fake Windows SDK host dir, both
// under temp dirs, and returns a Checker whose platformChecks will take the
// SDK >= 26100 engine-patch branches.
func sdkEngineFixture(t *testing.T, version string) *Checker {
	t.Helper()
	progFiles := t.TempDir()
	sdkDir := filepath.Join(progFiles, "Windows Kits", "10", "Include", "10.0.26100.0")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ProgramFiles(x86)", progFiles)
	t.Setenv("APPDATA", t.TempDir())
	sacBatchFake(t, "0", 0)
	return &Checker{EngineSourcePath: testsupport.FakeEngineTree(t, testsupport.WithVersion(version))}
}

func TestPlatformChecks_SDK26100PatchBranches(t *testing.T) {
	for _, version := range []string{"5.4", "5.6"} {
		t.Run(version, func(t *testing.T) {
			c := sdkEngineFixture(t, version)
			results := c.platformChecks()
			for _, res := range results {
				if res.Name == "C4756 Patch" || res.Name == "NNERuntimeORT Patch" {
					return
				}
			}
			t.Fatalf("platformChecks() missing version patch for %s: %+v", version, results)
		})
	}
}

func TestFixCrossCompileToolchain_UsesCachedInstaller(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.6"))
	spec := toolchain.LookupToolchain("5.6")
	installer := filepath.Join(os.TempDir(), fmt.Sprintf("ludus-toolchain-%s.exe", spec.SDKVersion))
	if err := os.WriteFile(installer, []byte("installer"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINUX_MULTIARCH_ROOT", "")
	sacBatchFake(t, "", 0)

	res := (&Checker{EngineSourcePath: root, Fix: true}).checkToolchain()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "toolchain installer completed") {
		t.Fatalf("checkToolchain() with --fix = %+v", res)
	}
}

func TestDownloadFile_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := downloadFile(filepath.Join(t.TempDir(), "out.exe"), srv.URL); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("downloadFile() error = %v, want HTTP 500", err)
	}
}

// robocopyFake writes a robocopy.bat stub that returns the given exit code and
// restricts PATH to that stub dir so the fake wins over the real robocopy.exe.
func robocopyFake(t *testing.T, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("@echo off\r\nexit /b %d\r\n", exitCode)
	if err := os.WriteFile(filepath.Join(dir, "robocopy.bat"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// overlayFixture returns a checker whose overlay destination already contains
// the required DefaultGameData.uasset so overlayLyraContent can report success.
func overlayFixture(t *testing.T) *Checker {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "Lyra")
	content := filepath.Join(dst, "Content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(content, "DefaultGameData.uasset"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Checker{EngineSourcePath: filepath.Dir(dst)}
}

func TestOverlayLyraContent_OnWindows(t *testing.T) {
	robocopyFake(t, 0)
	c := overlayFixture(t)
	res := c.overlayLyraContent(t.TempDir(), filepath.Join(c.EngineSourcePath, "Lyra"))
	if !res.Passed || !strings.Contains(res.Message, "overlaid from") {
		t.Fatalf("overlayLyraContent() = %+v", res)
	}
}

func TestOverlayLyraContent_GameDataStillMissing(t *testing.T) {
	robocopyFake(t, 0)
	dst := filepath.Join(t.TempDir(), "Lyra")
	res := (&Checker{}).overlayLyraContent(t.TempDir(), dst)
	if res.Passed || !strings.Contains(res.Message, "still missing") {
		t.Fatalf("overlayLyraContent() = %+v", res)
	}
}

func TestOverlayLyraContent_SourceMissing(t *testing.T) {
	res := (&Checker{}).overlayLyraContent(filepath.Join(t.TempDir(), "missing"), t.TempDir())
	if res.Passed || !strings.Contains(res.Message, "content source path does not exist") {
		t.Fatalf("overlayLyraContent() = %+v", res)
	}
}

func TestRobocopyOverlay_ExitCodes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		exit    int
		wantErr bool
	}{
		{"success", 0, false},
		{"minor error", 7, false},
		{"fatal error", 9, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			robocopyFake(t, tt.exit)
			err := (&Checker{}).robocopyOverlay(t.TempDir(), t.TempDir())
			if (err != nil) != tt.wantErr {
				t.Fatalf("robocopyOverlay() exit=%d err=%v, wantErr=%v", tt.exit, err, tt.wantErr)
			}
		})
	}
}

func TestFixMissingPluginContent_OverlayBranch(t *testing.T) {
	robocopyFake(t, 0)
	lyraDir := t.TempDir()
	content := filepath.Join(lyraDir, "Content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(content, "DefaultGameData.uasset"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := lyraContentState{lyraDir: lyraDir, missingPlugins: []string{"ShooterCore"}}
	res := (&Checker{Fix: true}).fixMissingPluginContent(s, t.TempDir())
	if !res.Passed {
		t.Fatalf("fixMissingPluginContent() = %+v", res)
	}
}
