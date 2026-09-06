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
		Required: &toolchain.ToolchainSpec{SDKVersion: "v26", DirPrefix: "v26_clang-20", InstallerURL: "http://example.invalid"},
	})
	if got.Passed || !strings.Contains(got.Message, "7z not found") {
		t.Fatalf("got %+v, want 7z-missing failure", got)
	}
}

func TestInstallUnixCrossCompileToolchain_DownloadFailure(t *testing.T) {
	stubUnix7z(t, "")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	got := installUnixCrossCompileToolchain(t.TempDir(), toolchain.CheckResult{
		Required: &toolchain.ToolchainSpec{SDKVersion: uniqueSDKVersion(t), DirPrefix: "v26_clang-20", InstallerURL: ts.URL},
	})
	if got.Passed || !strings.Contains(got.Message, "failed to download") {
		t.Fatalf("got %+v, want download failure", got)
	}
}

func TestInstallUnixCrossCompileToolchain_ExtractsArchive(t *testing.T) {
	root := t.TempDir()
	stubUnix7z(t, filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fake-installer"))
	}))
	t.Cleanup(ts.Close)

	got := installUnixCrossCompileToolchain(root, toolchain.CheckResult{
		Required: &toolchain.ToolchainSpec{SDKVersion: uniqueSDKVersion(t), DirPrefix: "v26_clang-20", InstallerURL: ts.URL},
	})
	if !got.Passed || !strings.Contains(got.Message, "extracted to") {
		t.Fatalf("got %+v, want successful extract", got)
	}
}

func TestInstallUnixCrossCompileToolchain_ExtractMissingDir(t *testing.T) {
	stubUnix7z(t, "")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fake-installer"))
	}))
	t.Cleanup(ts.Close)

	got := installUnixCrossCompileToolchain(t.TempDir(), toolchain.CheckResult{
		Required: &toolchain.ToolchainSpec{SDKVersion: uniqueSDKVersion(t), DirPrefix: "v26_clang-20", InstallerURL: ts.URL},
	})
	if got.Passed || !strings.Contains(got.Message, "was not found") {
		t.Fatalf("got %+v, want missing-directory failure", got)
	}
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

func stubUnix7z(t *testing.T, createDir string) {
	t.Helper()
	script := "#!/bin/sh\n"
	if createDir != "" {
		script += "mkdir -p " + createDir + "\n"
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "7z")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func uniqueSDKVersion(t *testing.T) string {
	t.Helper()
	return "v26-test-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + filepath.Base(t.TempDir())
}
