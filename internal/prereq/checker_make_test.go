package prereq

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/jpvelasco/ludus/internal/wrapper"
)

func TestCheckMakeForWrapper_SkipsWhenMakeNotUsed(t *testing.T) {
	// Targets that cross-compile (never shell out to make) must skip the check
	// and pass without a warning, regardless of host OS. linux/arm64 and
	// windows/amd64 cross-compile; on a non-Linux host even linux/amd64 skips.
	c := &Checker{}
	cases := [][2]string{
		{"linux", "arm64"},
		{"windows", "amd64"},
	}
	if runtime.GOOS != "linux" {
		cases = append(cases, [2]string{"linux", "amd64"})
	}

	for _, tc := range cases {
		res := c.checkMakeForWrapper(tc[0], tc[1])
		if !res.Passed || res.Warning {
			t.Errorf("checkMakeForWrapper(%q, %q): want clean skip-pass, got passed=%v warning=%v msg=%q",
				tc[0], tc[1], res.Passed, res.Warning, res.Message)
		}
	}
}

func TestCheckMakeForWrapper_NativeLinuxAmd64(t *testing.T) {
	// The make check only does real work for a native linux/amd64 build on a
	// Linux host; elsewhere UsesMake is false and the branch is unreachable.
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native linux/amd64 make check only runs on a linux/amd64 host")
	}

	res := (&Checker{}).checkMakeForWrapper("linux", "amd64")
	// The check passes when make is on PATH, OR when a prebuilt wrapper binary is
	// cached (no build needed, so make is irrelevant). It only fails when make is
	// absent AND nothing is cached.
	_, lookErr := exec.LookPath("make")
	wantPass := lookErr == nil || wrapper.IsBinaryCached("linux", "amd64")
	if res.Passed != wantPass {
		t.Errorf("checkMakeForWrapper(linux, amd64): passed=%v, want %v: %s",
			res.Passed, wantPass, res.Message)
	}
}

func TestCheckMakeForWrapper_SkipsWhenCached(t *testing.T) {
	// On a native linux/amd64 host, a cached wrapper binary must make the check
	// pass even if make were absent (EnsureBinary skips the build on a cache
	// hit). We can't remove make from PATH here, so assert the skip message and
	// pass when a binary happens to be cached; otherwise the branch is covered
	// by the make-present path.
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("cache-skip branch only reachable on a linux/amd64 host")
	}
	if !wrapper.IsBinaryCached("linux", "amd64") {
		t.Skip("no cached wrapper binary on this host; cache-skip branch not exercised")
	}

	res := (&Checker{}).checkMakeForWrapper("linux", "amd64")
	if !res.Passed {
		t.Errorf("checkMakeForWrapper with cached binary: want pass, got fail: %s", res.Message)
	}
}

func TestCheckWrapperBuildReady_Composition(t *testing.T) {
	// CheckWrapperBuildReady currently composes exactly the make check.
	res := (&Checker{}).CheckWrapperBuildReady("linux", "arm64")
	if len(res) != 1 {
		t.Fatalf("CheckWrapperBuildReady: got %d results, want 1", len(res))
	}
	if res[0].Name != "make" {
		t.Errorf("CheckWrapperBuildReady: result name = %q, want %q", res[0].Name, "make")
	}
}
