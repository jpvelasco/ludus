package dockerbuild

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/runner"
)

func TestScriptPreamble(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{}, r)
	got := b.scriptPreamble()

	assertContains(t, got, []string{
		"#!/bin/bash",
		"set -e",
		"useradd",           // UE 5.7+ refuses root on x86_64
		"su -p ue",          // preserve env when switching user
		"bash /build.sh",    // exec as ue user
		"HOME=/home/ue",     // su -p keeps HOME=/root
		"Build/Scripts/obj", // AutomationTool C# compilation
		"*.sym",             // pre-built .sym files for linker
	})

	// NuGet workaround is NOT in the preamble (moved to container -e args)
	if strings.Contains(got, "NuGetAuditLevel") {
		t.Error("NuGet workaround should not be in preamble (use envArgs instead)")
	}
}

func TestScriptPreamble_ZenMountParentChown(t *testing.T) {
	r := runner.NewRunner(false, false)

	// When a ZenStore mount is configured, Docker auto-creates the bind-mount
	// parents (/home/ue/.config and /home/ue/.config/Epic) owned by root. The
	// preamble must chown them to ue, or UAT (running as ue) fails creating its
	// sibling config dir /home/ue/.config/Unreal Engine (#340).
	withZen := NewDockerGameBuilder(DockerGameOptions{
		DDCMode:    ddc.ModeLocal,
		DDCPath:    "/tmp/ddc",
		DDCZenPath: "/tmp/zen",
	}, r)
	got := withZen.scriptPreamble()
	for _, want := range []string{
		"chown ue:ue /home/ue/.config",
		"/home/ue/.config/Epic",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("preamble with Zen mount should chown the mount parents; missing %q\ngot:\n%s", want, got)
		}
	}

	// Without a Zen mount there is no root-owned .config tree to fix, so the
	// preamble must not touch /home/ue/.config.
	noZen := NewDockerGameBuilder(DockerGameOptions{
		DDCMode: ddc.ModeLocal,
		DDCPath: "/tmp/ddc",
	}, r)
	if strings.Contains(noZen.scriptPreamble(), "/home/ue/.config") {
		t.Error("preamble without a Zen mount should not reference /home/ue/.config")
	}
}

func TestScriptPreamble_RecursiveProjectChown(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{}, r)
	got := b.scriptPreamble()

	// Every /project chown must be recursive. The project is the user's source
	// tree (with subdirs like Config/ that UAT's sed -i edits as the ue user);
	// a non-recursive chown leaves root-owned subdirs and the build fails with
	// "sed: couldn't open temporary file .../Config/...". /engine stays
	// non-recursive (copy-up cost), but /project must be -R in both the
	// new-user and pre-existing-ue branches.
	for line := range strings.SplitSeq(got, "\n") {
		if strings.Contains(line, "/project") && strings.Contains(line, "chown") &&
			!strings.Contains(line, "chown -R") {
			t.Errorf("preamble chowns /project non-recursively: %q", strings.TrimSpace(line))
		}
	}
	if !strings.Contains(got, "chown -R ue:ue /project") {
		t.Errorf("preamble must recursively chown /project (got:\n%s)", got)
	}
}

func TestScriptPreamble_InstallsRuntimeDeps(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{EngineVersion: "5.7"}, r)
	got := b.scriptPreamble()

	if !strings.Contains(got, "ldconfig") {
		t.Error("preamble should use ldconfig to check for missing libs")
	}

	// The preamble must include every package from AptRuntimePackages (single source of truth).
	for _, pkg := range AptRuntimePackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("preamble should install %q for UnrealEditor-Cmd runtime deps", pkg)
		}
	}

	// The preamble must fail fast if apt-get install fails, not silently continue
	// through a multi-hour compile only to crash at cook.
	if !strings.Contains(got, "exit 1") {
		t.Error("preamble must fail fast on install failure (exit 1)")
	}
}

func TestEnvArgs(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name      string
		opts      DockerGameOptions
		wantNuGet bool
	}{
		{
			name:      "empty version gets NuGet env",
			opts:      DockerGameOptions{},
			wantNuGet: true,
		},
		{
			name:      "5.6 gets NuGet env",
			opts:      DockerGameOptions{EngineVersion: "5.6"},
			wantNuGet: true,
		},
		{
			name:      "5.7 skips NuGet env",
			opts:      DockerGameOptions{EngineVersion: "5.7"},
			wantNuGet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			args := b.envArgs()
			hasNuGet := false
			for _, a := range args {
				if strings.Contains(a, "NuGetAuditLevel") {
					hasNuGet = true
				}
			}
			if hasNuGet != tt.wantNuGet {
				t.Errorf("NuGet env arg present = %v, want %v; args = %v", hasNuGet, tt.wantNuGet, args)
			}
		})
	}
}
