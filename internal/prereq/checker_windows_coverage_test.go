//go:build windows

package prereq

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindHighestSDKBuild(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"10.0.19041.0", "10.0.26100.0", "10.0.bad.0", "short"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "10.0.99999.0"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	name, build, err := findHighestSDKBuild(root)
	if err != nil || name != "10.0.26100.0" || build != 26100 {
		t.Fatalf("findHighestSDKBuild() = (%q, %d, %v)", name, build, err)
	}
}

func TestFindHighestSDKBuildFailures(t *testing.T) {
	tests := []struct {
		name string
		path func(*testing.T) string
		want string
	}{
		{"missing directory", func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") }, "cannot read"},
		{"no versions", func(t *testing.T) string { return t.TempDir() }, "no Windows SDK versions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := findHighestSDKBuild(tt.path(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckWindowsSDK(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantBuild   int
		wantPassed  bool
		wantWarning bool
	}{
		{"legacy", "10.0.22621.0", 22621, true, false},
		{"new warning", "10.0.26100.0", 26100, true, true},
		{"missing", "", 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("ProgramFiles(x86)", root)
			if tt.version != "" {
				dir := filepath.Join(root, "Windows Kits", "10", "Include", tt.version)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got, build := (&Checker{}).checkWindowsSDK()
			if build != tt.wantBuild || got.Passed != tt.wantPassed || got.Warning != tt.wantWarning {
				t.Fatalf("checkWindowsSDK() = (%+v, %d)", got, build)
			}
		})
	}
}

func TestBuildSACMessage(t *testing.T) {
	tests := []struct {
		name    string
		blocked []string
		want    []string
	}{
		{"no blocks", nil, []string{"enforcement mode", "To fix: Turn off SAC"}},
		{"few blocks", []string{"one.dll", "two.dll"}, []string{"Recently blocked files (2)", "two.dll"}},
		{"truncates blocks", []string{"1", "2", "3", "4", "5", "6", "7"}, []string{"Recently blocked files (7)", "... and 2 more"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSACMessage("enforcement", tt.blocked)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("buildSACMessage() missing %q", want)
				}
			}
		})
	}
}
func TestC4756StateAndResults(t *testing.T) {
	tests := []struct {
		name        string
		fix         bool
		contents    []string
		wantPassed  bool
		wantWarning bool
		wantMessage string
	}{
		{"all absent", false, nil, true, true, "source files not found"},
		{"needs patch", false, []string{"#include <math.h>\n", ""}, false, false, "suppression missing"},
		{"already patched", false, []string{"#pragma warning(disable: 4756)\n#include <math.h>\n", ""}, true, false, "suppression present"},
		{"applies patch", true, []string{"// header\n#include <math.h>\n", ""}, true, false, "patched 1 file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testC4756State(t, tt.fix, tt.contents, tt.wantPassed, tt.wantWarning, tt.wantMessage)
		})
	}
}

func TestC4756PatchFailures(t *testing.T) {
	root := t.TempDir()
	writeC4756File(t, root, 0, "no include here")
	got := (&Checker{EngineSourcePath: root, Fix: true}).checkC4756Patch()
	if got.Passed || !strings.Contains(got.Message, "could not find #include") {
		t.Fatalf("missing marker result = %+v", got)
	}

	dirPath := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	got = *applyC4756Patch(dirPath, "#include <x>", "#pragma warning(disable: 4756)")
	if got.Passed || !strings.Contains(got.Message, "failed to write") {
		t.Fatalf("write failure result = %+v", got)
	}
}

func TestC4756ResultStates(t *testing.T) {
	tests := []struct {
		name  string
		state c4756State
		want  string
	}{
		{"patched", c4756State{patched: 2}, "patched 2 file"},
		{"present", c4756State{alreadyPatched: 1}, "suppression present"},
		{"unpatched", c4756State{firstUnpatched: "file.cpp"}, "file.cpp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&Checker{}).c4756Result(tt.state); !strings.Contains(got.Message, tt.want) {
				t.Fatalf("c4756Result() = %+v", got)
			}
		})
	}
}

func TestNNERuntimeORTPatch(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		fix         bool
		wantPassed  bool
		wantMessage string
	}{
		{"missing file", "", false, false, "cannot read"},
		{"already patched", `PublicDefinitions.Add("INITGUID");`, false, true, "definition present"},
		{"fix disabled", `PublicDefinitions.Add("ORT_USE_NEW_DXCORE_FEATURES");`, false, false, "run with --fix"},
		{"marker absent", "using UnrealBuildTool;", true, false, "could not find"},
		{"patch marker line", "    " + `PublicDefinitions.Add("ORT_USE_NEW_DXCORE_FEATURES");` + "\nnext\n", true, true, "patched"},
		{"patch final line", `PublicDefinitions.Add("ORT_USE_NEW_DXCORE_FEATURES");`, true, true, "patched"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testORTPatch(t, tt.content, tt.fix, tt.wantPassed, tt.wantMessage)
		})
	}
}

func TestApplyORTPatchWriteFailure(t *testing.T) {
	path := t.TempDir()
	got := applyORTPatch(path)
	if got.Passed || !strings.Contains(got.Message, "cannot read") {
		t.Fatalf("applyORTPatch() = %+v", got)
	}
}

func TestBuildConfigXMLFor(t *testing.T) {
	tests := []struct {
		name     string
		compiler string
		want     string
	}{
		{"version only", "", "<CompilerVersion>14.38</CompilerVersion>"},
		{"compiler", "VisualStudio2026", "<Compiler>VisualStudio2026</Compiler>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildConfigXMLFor("14.38", tt.compiler)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("buildConfigXMLFor() = %q", got)
			}
		})
	}
}

func TestExistingMSVCConfigAndHints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "BuildConfiguration.xml")
	ok, _ := checkExistingMSVCConfig(path, "<CompilerVersion>14.44</CompilerVersion>", "14.44", "")
	if ok {
		t.Fatal("missing config reported correct")
	}
	writeTestFile(t, path, buildConfigXMLFor("14.44", ""))
	ok, msg := checkExistingMSVCConfig(path, "<CompilerVersion>14.44</CompilerVersion>", "14.44", "")
	if !ok || !strings.Contains(msg, "pins MSVC 14.44") {
		t.Fatalf("existing config = (%v, %q)", ok, msg)
	}
	ok, _ = checkExistingMSVCConfig(path, "<CompilerVersion>14.44</CompilerVersion>", "14.44", "VisualStudio2026")
	if ok {
		t.Fatal("config without requested compiler reported correct")
	}
	if got := msvcConfigHint(path, "14.44", "VisualStudio2026"); got.Passed || !strings.Contains(got.Message, "Compiler/CompilerVersion") {
		t.Fatalf("msvcConfigHint() = %+v", got)
	}
}

func TestFixMSVCConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "BuildConfiguration.xml")
	got := fixMSVCConfig(filepath.Dir(path), path, "14.44", "VisualStudio2026")
	if !got.Passed || !strings.Contains(got.Message, "Compiler=VisualStudio2026") {
		t.Fatalf("fixMSVCConfig() = %+v", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "<Compiler>VisualStudio2026</Compiler>") {
		t.Fatalf("config content = %q, %v", data, err)
	}
	got = fixMSVCConfig(filepath.Dir(path), path, "14.44", "VisualStudio2026")
	if !got.Passed {
		t.Fatalf("idempotent fixMSVCConfig() = %+v", got)
	}
}

func TestFixMSVCConfigFailures(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	writeTestFile(t, file, "occupied")
	tests := []struct {
		name string
		dir  string
		path string
		want string
	}{
		{"mkdir", filepath.Join(file, "child"), filepath.Join(file, "child", "config.xml"), "failed to create directory"},
		{"write", root, root, "failed to write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixMSVCConfig(tt.dir, tt.path, "14.38", "")
			if got.Passed || !strings.Contains(got.Message, tt.want) {
				t.Fatalf("fixMSVCConfig() = %+v", got)
			}
		})
	}
}

func TestCheckMSVCToolchainConfig(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		fix         bool
		preexisting bool
		wantPassed  bool
	}{
		{"missing", "5.6", false, false, false},
		{"create legacy", "5.6", true, false, true},
		{"create newer", "5.8", true, false, true},
		{"existing", "5.8", false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appData := t.TempDir()
			t.Setenv("APPDATA", appData)
			path := filepath.Join(appData, "Unreal Engine", "UnrealBuildTool", "BuildConfiguration.xml")
			if tt.preexisting {
				writeTestFile(t, path, buildConfigXMLFor("14.44.35207", "VisualStudio2026"))
			}
			got := (&Checker{EngineVersion: tt.version, Fix: tt.fix}).checkMSVCToolchainConfig()
			if got.Passed != tt.wantPassed {
				t.Fatalf("checkMSVCToolchainConfig() = %+v", got)
			}
		})
	}
}

func TestCopyWithProgress(t *testing.T) {
	var dst strings.Builder
	if err := copyWithProgress(strings.NewReader("payload"), &dst, 7); err != nil || dst.String() != "payload" {
		t.Fatalf("copyWithProgress() = (%q, %v)", dst.String(), err)
	}
	if err := copyWithProgress(errorReader{}, &dst, 0); !errors.Is(err, errTestRead) {
		t.Fatalf("read error = %v", err)
	}
	if err := copyWithProgress(strings.NewReader("x"), errorWriter{}, 0); !errors.Is(err, errTestWrite) {
		t.Fatalf("write error = %v", err)
	}
}

var (
	errTestRead  = errors.New("test read failure")
	errTestWrite = errors.New("test write failure")
)

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errTestRead }

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errTestWrite }

func testC4756State(t *testing.T, fix bool, contents []string, wantPassed, wantWarning bool, wantMessage string) {
	t.Helper()
	root := t.TempDir()
	if contents != nil {
		writeC4756File(t, root, 0, contents[0])
	}
	got := (&Checker{EngineSourcePath: root, Fix: fix}).checkC4756Patch()
	if got.Passed != wantPassed || got.Warning != wantWarning || !strings.Contains(got.Message, wantMessage) {
		t.Fatalf("checkC4756Patch() = %+v", got)
	}
	if fix && contents != nil {
		assertFileContains(t, filepath.Join(root, c4756Files[0].relPath), "#pragma warning(disable: 4756)")
	}
}

func testORTPatch(t *testing.T, content string, fix, wantPassed bool, wantMessage string) {
	t.Helper()
	root := t.TempDir()
	path := ortBuildPath(root)
	if content != "" {
		writeTestFile(t, path, content)
	}
	got := (&Checker{EngineSourcePath: root, Fix: fix}).checkNNERuntimeORTPatch()
	if got.Passed != wantPassed || !strings.Contains(got.Message, wantMessage) {
		t.Fatalf("checkNNERuntimeORTPatch() = %+v", got)
	}
	if fix && wantPassed {
		assertFileContains(t, path, `PublicDefinitions.Add("INITGUID");`)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), want) {
		t.Fatalf("file content = %q, %v; want %q", data, err, want)
	}
}
func writeC4756File(t *testing.T, root string, index int, content string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, c4756Files[index].relPath), content)
}

func ortBuildPath(root string) string {
	return filepath.Join(root, "Engine", "Plugins", "NNE", "NNERuntimeORT",
		"Source", "NNERuntimeORT", "NNERuntimeORT.Build.cs")
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
