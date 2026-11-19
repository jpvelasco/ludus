//go:build !windows

package prereq

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/toolchain"
)

func TestInstallUnixCrossCompileToolchain_NoURL(t *testing.T) {
	got := installUnixCrossCompileToolchain(t.TempDir(), toolchain.CheckResult{})
	if !got.Passed || !got.Warning {
		t.Fatalf("got %+v, want pass+warning", got)
	}
}

func TestInstallUnixCrossCompileToolchain_NoEnginePath(t *testing.T) {
	got := installUnixCrossCompileToolchain("", toolchain.CheckResult{
		Required: &toolchain.ToolchainSpec{InstallerURL: "http://example.invalid", DirPrefix: "v26_clang-20"},
	})
	if got.Passed || !strings.Contains(got.Message, "engine source path") {
		t.Fatalf("got %+v, want engine-path failure", got)
	}
}

func TestInstallUnixCrossCompileToolchain_Missing7z(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	got := installUnixCrossCompileToolchain(t.TempDir(), toolchain.CheckResult{
		Required: &toolchain.ToolchainSpec{SDKVersion: "v26", DirPrefix: "v26_clang-20", InstallerURL: "http://example.invalid/" + testToolchainDir + ".exe"},
	})
	if got.Passed || !strings.Contains(got.Message, "7z not found") || !strings.Contains(got.Message, "Linux_x64/"+testToolchainDir) {
		t.Fatalf("got %+v, want 7z-missing failure", got)
	}
}

func TestInstallUnixCrossCompileToolchain_DownloadFailure(t *testing.T) {
	stubUnix7z(t, archiveFlat)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	got := installUnixCrossCompileToolchain(t.TempDir(), testToolchainCheck(t, ts.URL+"/"+testToolchainDir+".exe"))
	if got.Passed || !strings.Contains(got.Message, "failed to download") {
		t.Fatalf("got %+v, want download failure", got)
	}
}

// Epic's installer archive has no top-level version folder (#838), so the
// toolchain must be extracted into Linux_x64/<installer basename>/ rather than
// spilling into Linux_x64/ itself.
func TestInstallUnixCrossCompileToolchain_ExtractsIntoVersionDir(t *testing.T) {
	for _, layout := range []string{archiveFlat, archiveNested} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			stubUnix7z(t, layout)
			tc := testToolchainCheck(t, serveInstaller(t)+"/"+testToolchainDir+".exe")

			got := installUnixCrossCompileToolchain(root, tc)
			if !got.Passed || !strings.Contains(got.Message, "extracted to") {
				t.Fatalf("got %+v, want successful extract", got)
			}
			assertSDKDirEntries(t, root, []string{testToolchainDir})
			assertExists(t, filepath.Join(sdkDirFor(root), testToolchainDir, toolchainArchDir), true)
			assertExists(t, cachedInstallerPath(tc.Required), false)
		})
	}
}

func TestInstallUnixCrossCompileToolchain_ExtractFailures(t *testing.T) {
	tests := []struct {
		name, layout, installer, want string
	}{
		{"empty archive", archiveEmpty, testToolchainDir + ".exe", "does not contain " + toolchainArchDir},
		{"7z error", archiveFail, testToolchainDir + ".exe", "failed to extract"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			stubUnix7z(t, tt.layout)
			tc := testToolchainCheck(t, serveInstaller(t)+"/"+tt.installer)

			got := installUnixCrossCompileToolchain(root, tc)
			if got.Passed || !strings.Contains(got.Message, tt.want) {
				t.Fatalf("got %+v, want failure containing %q", got, tt.want)
			}
			assertSDKDirEntries(t, root, nil)
			assertExists(t, cachedInstallerPath(tc.Required), true)
		})
	}
}

func TestInstallUnixCrossCompileToolchain_UnexpectedInstallerName(t *testing.T) {
	stubUnix7z(t, archiveFlat)
	tc := testToolchainCheck(t, serveInstaller(t)+"/something-else.exe")

	got := installUnixCrossCompileToolchain(t.TempDir(), tc)
	if got.Passed || !strings.Contains(got.Message, "does not match") {
		t.Fatalf("got %+v, want installer-name failure", got)
	}
	assertExists(t, cachedInstallerPath(tc.Required), false)
}

func TestFindExtractedToolchain(t *testing.T) {
	dir := t.TempDir()
	if got := findExtractedToolchain(dir, "v26_clang-20"); got != "" {
		t.Fatalf("empty dir = %q, want empty", got)
	}
	want := filepath.Join(dir, "v26_clang-20.1.8-rockylinux8")
	if err := os.Mkdir(want, 0755); err != nil {
		t.Fatal(err)
	}
	if got := findExtractedToolchain(dir, "v26_clang-20"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

const (
	testToolchainDir = "v26_clang-20.1.8-rockylinux8"
	archiveFlat      = "flat"
	archiveNested    = "nested"
	archiveEmpty     = "empty"
	archiveFail      = "fail"
)

// stubUnix7z puts a fake 7z on PATH that writes the given archive layout into
// its -o directory. "flat" mirrors Epic's real installer (no version folder).
func stubUnix7z(t *testing.T, layout string) {
	t.Helper()
	script := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in -o*) out=\"${a#-o}\";; esac; done\n"
	switch layout {
	case archiveFlat:
		script += "mkdir -p \"$out/" + toolchainArchDir + "\" && touch \"$out/ToolchainVersion.txt\"\n"
	case archiveNested:
		script += "mkdir -p \"$out/" + testToolchainDir + "/" + toolchainArchDir + "\"\n"
	case archiveFail:
		script += "echo 'ERROR: corrupt archive' >&2\nexit 2\n"
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "7z")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func serveInstaller(t *testing.T) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fake-installer"))
	}))
	t.Cleanup(ts.Close)
	return ts.URL
}

func testToolchainCheck(t *testing.T, installerURL string) toolchain.CheckResult {
	t.Helper()
	spec := &toolchain.ToolchainSpec{SDKVersion: uniqueSDKVersion(t), DirPrefix: "v26_clang-20", InstallerURL: installerURL}
	t.Cleanup(func() { _ = os.Remove(cachedInstallerPath(spec)) })
	return toolchain.CheckResult{Required: spec}
}

func sdkDirFor(root string) string {
	return filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
}

func assertSDKDirEntries(t *testing.T, root string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(sdkDirFor(root))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Linux_x64 entries = %v, want %v", got, want)
	}
}

func assertExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if got := err == nil; got != want {
		t.Fatalf("exists(%s) = %v, want %v", path, got, want)
	}
}

func uniqueSDKVersion(t *testing.T) string {
	t.Helper()
	return "v26-test-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + filepath.Base(t.TempDir())
}
