//go:build windows

package prereq

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckPluginDLLDepsVersions(t *testing.T) {
	tests := []struct {
		name, version string
		want          int
	}{{name: "unknown"}, {name: "malformed", version: "five"}, {name: "bad minor", version: "5.next"}, {name: "unaffected", version: "5.5"}, {name: "dataflow", version: "5.6", want: 1}, {name: "crypto", version: "5.7", want: 1}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&Checker{EngineSourcePath: t.TempDir(), EngineVersion: tt.version}).checkPluginDLLDeps()
			if len(got) != tt.want {
				t.Fatalf("got %d results, want %d: %#v", len(got), tt.want, got)
			}
			if tt.want == 1 && (!got[0].Passed || !got[0].Warning) {
				t.Errorf("got %#v, want passing warning", got[0])
			}
		})
	}
}

func TestPluginDLLDiscovery(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
	pluginMkdir(t, src)
	pluginMkdir(t, dst)
	pluginWrite(t, filepath.Join(src, "missing.dll"))
	pluginWrite(t, filepath.Join(src, "present.dll"))
	pluginWrite(t, filepath.Join(dst, "present.dll"))
	pluginWrite(t, filepath.Join(dst, "first.dll"))
	pluginWrite(t, filepath.Join(dst, "first.pdb"))
	pluginWrite(t, filepath.Join(dst, "second.pdb"))
	if got := findMissingDLLs(src, dst, []string{"absent.dll", "missing.dll", "present.dll"}); !slices.Equal(got, []string{"missing.dll"}) {
		t.Errorf("missing = %v", got)
	}
	want := []string{"first.dll", "first.pdb", "second.pdb"}
	if got := findStaleFiles(dst, []string{"first.dll", "second.dll", "absent.dll"}); !slices.Equal(got, want) {
		t.Errorf("stale = %v, want %v", got, want)
	}
}

func TestApplyPluginDLLFix(t *testing.T) {
	tests := []struct {
		name                                      string
		fix, source, destination, passed, warning bool
		message                                   string
	}{{name: "not built", passed: true, warning: true, message: "plugin not built"}, {name: "disabled", source: true, message: "run with --fix"}, {name: "present", source: true, destination: true, passed: true, message: "present"}, {name: "copy", fix: true, source: true, passed: true, message: "copied 1 DLL"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testApplyPluginFix(t, tt.name, tt.fix, tt.source, tt.destination, tt.passed, tt.warning, tt.message)
		})
	}
}

func testApplyPluginFix(t *testing.T, name string, fixEnabled, source, destination, passed, warning bool, message string) {
	t.Helper()
	root := t.TempDir()
	fix := coveragePluginFix()
	src := filepath.Join(root, fix.pluginRelPath)
	dst := filepath.Join(root, "Engine", "Binaries", "Win64")
	if source {
		pluginMkdir(t, src)
		pluginWrite(t, filepath.Join(src, fix.dllNames[0]))
	}
	if destination {
		pluginMkdir(t, dst)
		pluginWrite(t, filepath.Join(dst, fix.dllNames[0]))
	} else if fixEnabled {
		pluginMkdir(t, dst)
	}
	got := (&Checker{EngineSourcePath: root, Fix: fixEnabled}).applyPluginDLLFix(fix)
	pluginResult(t, got, passed, warning, message)
	if name == "copy" {
		pluginExists(t, filepath.Join(dst, fix.dllNames[0]))
	}
}

func TestApplyPluginDLLFixFailures(t *testing.T) {
	tests := []struct {
		name, message string
		arrange       func(*testing.T, string, string, string)
	}{{name: "read", message: "failed to read", arrange: pluginReadFailure}, {name: "write", message: "failed to write", arrange: pluginWriteFailure}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			fix := coveragePluginFix()
			src := filepath.Join(root, fix.pluginRelPath)
			dst := filepath.Join(root, "Engine", "Binaries", "Win64")
			tt.arrange(t, src, dst, fix.dllNames[0])
			pluginResult(t, (&Checker{EngineSourcePath: root, Fix: true}).applyPluginDLLFix(fix), false, false, tt.message)
		})
	}
}

func TestCleanupPluginDLLFix(t *testing.T) {
	tests := []struct {
		name                   string
		enabled, stale, passed bool
		count                  int
		message                string
	}{{name: "none"}, {name: "disabled", stale: true, count: 1, message: "run with --fix"}, {name: "remove", enabled: true, stale: true, passed: true, count: 1, message: "removed 2 stale"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := t.TempDir()
			fix := coveragePluginFix()
			if tt.stale {
				pluginWrite(t, filepath.Join(dst, fix.dllNames[0]))
				pluginWrite(t, filepath.Join(dst, "TestPlugin.pdb"))
			}
			got := (&Checker{Fix: tt.enabled}).cleanupSingleFix(dst, fix)
			if len(got) != tt.count {
				t.Fatalf("got %d, want %d: %#v", len(got), tt.count, got)
			}
			if tt.count > 0 {
				pluginResult(t, got[0], tt.passed, false, tt.message)
			}
		})
	}
}

func TestCleanupPluginDLLRemovalFailure(t *testing.T) {
	dst := t.TempDir()
	fix := coveragePluginFix()
	stale := filepath.Join(dst, fix.dllNames[0])
	pluginMkdir(t, stale)
	pluginWrite(t, filepath.Join(stale, "child"))
	got := (&Checker{Fix: true}).cleanupSingleFix(dst, fix)
	if len(got) != 1 {
		t.Fatalf("got %#v", got)
	}
	pluginResult(t, got[0], false, false, "failed to remove stale")
}

func TestCleanupStaleDLLVersionGate(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "Engine", "Binaries", "Win64")
	pluginMkdir(t, dst)
	dataflow := findPluginFix(t, "Dataflow Plugin DLLs")
	crypto := findPluginFix(t, "PlatformCrypto Plugin DLLs")
	pluginWrite(t, filepath.Join(dst, dataflow.dllNames[0]))
	pluginWrite(t, filepath.Join(dst, crypto.dllNames[0]))
	got := (&Checker{EngineSourcePath: root}).cleanupStaleDLLs(6)
	if len(got) != 1 || !strings.Contains(got[0].Name, crypto.name) {
		t.Fatalf("got %#v", got)
	}
}

func TestIntSliceContainsCoverage(t *testing.T) {
	tests := []struct {
		list  []int
		value int
		want  bool
	}{{}, {list: []int{1, 2}, value: 3}, {list: []int{1, 2}, value: 2, want: true}}
	for _, tt := range tests {
		if got := intSliceContains(tt.list, tt.value); got != tt.want {
			t.Errorf("got %t, want %t", got, tt.want)
		}
	}
}
func coveragePluginFix() pluginDLLFix {
	return pluginDLLFix{name: "Test Plugin DLLs", description: "test", minorVersions: []int{6}, pluginRelPath: filepath.Join("Engine", "Plugins", "Test", "Binaries", "Win64"), dllNames: []string{"TestPlugin.dll"}}
}
func pluginReadFailure(t *testing.T, src, dst, dll string) {
	t.Helper()
	pluginMkdir(t, filepath.Join(src, dll))
	pluginMkdir(t, dst)
}
func pluginWriteFailure(t *testing.T, src, dst, dll string) {
	t.Helper()
	pluginMkdir(t, src)
	pluginWrite(t, filepath.Join(src, dll))
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("pluginWriteFailure: destination must not exist")
	}
}
func pluginMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
func pluginWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
}
func pluginResult(t *testing.T, got CheckResult, passed, warning bool, message string) {
	t.Helper()
	if got.Passed != passed || got.Warning != warning || !strings.Contains(got.Message, message) {
		t.Errorf("got %#v, want Passed=%t Warning=%t message %q", got, passed, warning, message)
	}
}
func pluginExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Stat(%q): %v", path, err)
	}
}
