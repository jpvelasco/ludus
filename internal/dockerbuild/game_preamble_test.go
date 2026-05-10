package dockerbuild

import (
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
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
