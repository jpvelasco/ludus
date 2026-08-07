//go:build linux

package prereq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wrapper"
)

// isolateWrapperCache points the wrapper cache at an empty temp dir so the
// IsBinaryCached probe is deterministic regardless of the host's real cache.
func isolateWrapperCache(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

// TestCheckMakeForWrapper_MakeOnPathPasses covers the "make found" tail of
// checkMakeForWrapper for a native linux/amd64 target on any host OS: a make
// stub on PATH and an empty wrapper cache must yield a clean pass.
func TestCheckMakeForWrapper_MakeOnPathPasses(t *testing.T) {
	isolateWrapperCache(t)
	testsupport.FakeTool(t, "make", testsupport.ToolBehavior{})

	res := (&Checker{}).checkMakeForWrapper("linux", "amd64")
	if !res.Passed || res.Warning {
		t.Fatalf("checkMakeForWrapper() = %+v, want clean pass", res)
	}
	if !strings.Contains(res.Message, "make found") {
		t.Fatalf("checkMakeForWrapper() message = %q, want 'make found'", res.Message)
	}
}

// TestCheckMakeForWrapper_MakeMissingFails covers the hard-failure branch: a
// native linux/amd64 target with no cached wrapper binary and no make on PATH.
func TestCheckMakeForWrapper_MakeMissingFails(t *testing.T) {
	isolateWrapperCache(t)
	t.Setenv("PATH", t.TempDir())

	res := (&Checker{}).checkMakeForWrapper("linux", "amd64")
	if res.Passed {
		t.Fatalf("checkMakeForWrapper() = %+v, want hard failure", res)
	}
	if !strings.Contains(res.Message, "make not found in PATH") {
		t.Fatalf("checkMakeForWrapper() message = %q, want 'make not found in PATH'", res.Message)
	}
}

// TestCheckMakeForWrapper_CachedBinarySkips covers the cache-hit branch: a
// prebuilt wrapper binary in the cache means make is not needed even though
// the target is native linux/amd64 and make is absent from PATH.
func TestCheckMakeForWrapper_CachedBinarySkips(t *testing.T) {
	isolateWrapperCache(t)
	t.Setenv("PATH", t.TempDir())

	cacheDir, err := wrapper.CacheDir()
	if err != nil {
		t.Fatalf("wrapper.CacheDir() error = %v", err)
	}
	binaryPath := wrapper.BinaryPath(cacheDir, "linux", "amd64")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !wrapper.IsBinaryCached("linux", "amd64") {
		t.Fatal("IsBinaryCached() = false, want true after seeding cache")
	}

	res := (&Checker{}).checkMakeForWrapper("linux", "amd64")
	if !res.Passed || res.Warning {
		t.Fatalf("checkMakeForWrapper() = %+v, want clean pass via cache", res)
	}
	if !strings.Contains(res.Message, "already cached") {
		t.Fatalf("checkMakeForWrapper() message = %q, want 'already cached'", res.Message)
	}
}
