//go:build windows

package prereq

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

// TestCleanupStaleDLLs_KeepsNativeDataflowDLLsOn57 pins the #746 bug: on
// UE 5.7+ the Dataflow DLL names in the 5.6 workaround table are native
// engine binaries. Without a provenance manifest recording ludus copying
// them, cleanup must leave every one of them alone.
func TestCleanupStaleDLLs_KeepsNativeDataflowDLLsOn57(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	dst := filepath.Join(root, "Engine", "Binaries", "Win64")
	pluginMkdir(t, dst)
	dataflow := findPluginFix(t, "Dataflow Plugin DLLs")
	for _, dll := range dataflow.dllNames {
		pluginWrite(t, filepath.Join(dst, dll))
	}

	got := (&Checker{EngineSourcePath: root, Fix: true}).cleanupStaleDLLs(7)
	for _, r := range got {
		if r.Passed {
			t.Errorf("cleanup must not remove native 5.7 binaries, got %#v", r)
		}
	}
	for _, dll := range dataflow.dllNames {
		pluginExists(t, filepath.Join(dst, dll))
	}
}

// TestCleanupStaleDLLs_RemovesManifestedPriorWorkaround covers the real stale
// case: a manifest records ludus copying Dataflow DLLs on 5.6, and the engine
// tree has since moved to 5.7 where the workaround no longer applies — so the
// owned copies are removed and the manifest follows.
func TestCleanupStaleDLLs_RemovesManifestedPriorWorkaround(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	dst := filepath.Join(root, "Engine", "Binaries", "Win64")
	pluginMkdir(t, dst)
	dataflow := findPluginFix(t, "Dataflow Plugin DLLs")
	for _, dll := range dataflow.dllNames {
		pluginWrite(t, filepath.Join(dst, dll))
	}
	writeTestManifest(t, dst, []pluginDLLCopy{{Fix: dataflow.name, File: dataflow.dllNames[0]}})

	got := (&Checker{EngineSourcePath: root, Fix: true}).cleanupStaleDLLs(7)
	if len(got) == 0 {
		t.Fatal("cleanup should report the manifested stale copy")
	}
	pluginResult(t, got[0], true, false, "removed 1 stale")
	if _, err := os.Stat(filepath.Join(dst, dataflow.dllNames[0])); err == nil {
		t.Error("manifested stale copy should be removed")
	}
	if _, err := os.Stat(filepath.Join(dst, dataflow.dllNames[1])); err != nil {
		t.Errorf("file not owned by the manifest must stay: %v", err)
	}
}

// TestCleanupStaleDLLs_KeepsCurrentFixManifestEntries covers 5.6 with a
// manifest: entries owned by the Dataflow fix (which applies on 5.6) are not
// stale and must stay on disk.
func TestCleanupStaleDLLs_KeepsCurrentFixManifestEntries(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.6.4"))
	dst := filepath.Join(root, "Engine", "Binaries", "Win64")
	pluginMkdir(t, dst)
	dataflow := findPluginFix(t, "Dataflow Plugin DLLs")
	pluginWrite(t, filepath.Join(dst, dataflow.dllNames[0]))
	writeTestManifest(t, dst, []pluginDLLCopy{{Fix: dataflow.name, File: dataflow.dllNames[0]}})

	got := (&Checker{EngineSourcePath: root, Fix: true}).cleanupStaleDLLs(6)
	if len(got) != 0 {
		t.Fatalf("cleanup on 5.6 should not report the current fix's copies, got %#v", got)
	}
	pluginExists(t, filepath.Join(dst, dataflow.dllNames[0]))
}

// TestCleanupStaleDLLs_PreviewWithoutFix covers the Fix=false preview: the
// stale manifested copies are reported as actionable but left on disk.
func TestCleanupStaleDLLs_PreviewWithoutFix(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	dst := filepath.Join(root, "Engine", "Binaries", "Win64")
	pluginMkdir(t, dst)
	dataflow := findPluginFix(t, "Dataflow Plugin DLLs")
	pluginWrite(t, filepath.Join(dst, dataflow.dllNames[0]))
	writeTestManifest(t, dst, []pluginDLLCopy{{Fix: dataflow.name, File: dataflow.dllNames[0]}})

	got := (&Checker{EngineSourcePath: root, Fix: false}).cleanupStaleDLLs(7)
	if len(got) != 1 {
		t.Fatalf("preview should report one actionable cleanup, got %#v", got)
	}
	pluginResult(t, got[0], false, false, "run with --fix")
	pluginExists(t, filepath.Join(dst, dataflow.dllNames[0]))
}

// TestApplyPluginDLLFixRecordsManifest verifies the copy path writes the
// provenance manifest alongside the copies, so future cleanup can tell
// ludus-owned files from native engine binaries.
func TestApplyPluginDLLFixRecordsManifest(t *testing.T) {
	root := t.TempDir()
	dataflow := findPluginFix(t, "Dataflow Plugin DLLs")
	src := filepath.Join(root, dataflow.pluginRelPath)
	dst := filepath.Join(root, "Engine", "Binaries", "Win64")
	pluginMkdir(t, src)
	pluginMkdir(t, dst)
	for _, dll := range dataflow.dllNames {
		pluginWrite(t, filepath.Join(src, dll))
	}

	got := (&Checker{EngineSourcePath: root, Fix: true}).applyPluginDLLFix(dataflow)
	pluginResult(t, got, true, false, "copied")

	manifest := readPluginDLLManifest(dst)
	if len(manifest.Copies) != len(dataflow.dllNames) {
		t.Fatalf("manifest = %#v, want %d entries", manifest.Copies, len(dataflow.dllNames))
	}
	for _, copy := range manifest.Copies {
		if copy.Fix != dataflow.name || !slices.Contains(dataflow.dllNames, copy.File) {
			t.Errorf("manifest entry %#v does not match the copied fix", copy)
		}
	}
}

// TestWritePluginDLLManifestEmptyRemovesFile pins the manifest lifecycle: when
// the last entry drops out, the manifest file itself is deleted.
func TestWritePluginDLLManifestEmptyRemovesFile(t *testing.T) {
	dst := t.TempDir()
	writeTestManifest(t, dst, []pluginDLLCopy{{Fix: "f", File: "x.dll"}})

	if err := writePluginDLLManifest(dst, nil); err != nil {
		t.Fatalf("writePluginDLLManifest() error = %v", err)
	}
	if _, err := os.Stat(pluginDLLManifestPath(dst)); !os.IsNotExist(err) {
		t.Errorf("empty manifest should be removed, stat err = %v", err)
	}
}

// TestReadPluginDLLManifestCorruptTreatedAsAbsent pins that an unparsable
// manifest never causes deletions — cleanup simply has no provenance.
func TestReadPluginDLLManifestCorruptTreatedAsAbsent(t *testing.T) {
	dst := t.TempDir()
	if err := os.WriteFile(pluginDLLManifestPath(dst), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if manifest := readPluginDLLManifest(dst); len(manifest.Copies) != 0 {
		t.Errorf("corrupt manifest should read as absent, got %#v", manifest.Copies)
	}
}

// writeTestManifest writes a provenance manifest into dst.
func writeTestManifest(t *testing.T, dst string, copies []pluginDLLCopy) {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(pluginDLLManifest{Copies: copies})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginDLLManifestPath(dst), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
