package dockerbuild

import (
	"runtime"
	"strings"
	"testing"
)

func TestResolveMaxJobs(t *testing.T) {
	t.Run("explicit value passes through", func(t *testing.T) {
		for _, n := range []int{1, 8, 16, 64} {
			if got := resolveMaxJobs(n); got != n {
				t.Errorf("resolveMaxJobs(%d) = %d, want %d", n, got, n)
			}
		}
	})

	t.Run("zero and negative auto-detect a positive value", func(t *testing.T) {
		for _, n := range []int{0, -1, -100} {
			got := resolveMaxJobs(n)
			if got < 1 {
				t.Errorf("resolveMaxJobs(%d) = %d, want >= 1 (auto-detected)", n, got)
			}
			// Auto-detect must never exceed the core count (RAM only lowers it).
			if got > runtime.NumCPU() {
				t.Errorf("resolveMaxJobs(%d) = %d, want <= NumCPU (%d)", n, got, runtime.NumCPU())
			}
		}
	})
}

func TestAutoMaxJobs(t *testing.T) {
	got := autoMaxJobs()
	if got < 1 {
		t.Fatalf("autoMaxJobs() = %d, want >= 1", got)
	}
	if got > runtime.NumCPU() {
		t.Errorf("autoMaxJobs() = %d, want <= NumCPU (%d) — RAM budget should only lower it", got, runtime.NumCPU())
	}
}

func TestGenerateEngineDockerfile(t *testing.T) {
	tests := []struct {
		name     string
		opts     DockerfileOptions
		contains []string
	}{
		{
			// MAX_JOBS auto-detects from the host when unset, so assert only the
			// base image here; the auto-detect value is covered by TestAutoMaxJobs.
			name: "defaults use ubuntu base",
			opts: DockerfileOptions{},
			contains: []string{
				"FROM ubuntu:22.04",
				"ARG MAX_JOBS=",
			},
		},
		{
			name: "custom base image and max jobs",
			opts: DockerfileOptions{MaxJobs: 16, BaseImage: "amazonlinux:2023"},
			contains: []string{
				"FROM amazonlinux:2023",
				"ARG MAX_JOBS=16",
			},
		},
		{
			name: "max jobs 1 is preserved",
			opts: DockerfileOptions{MaxJobs: 1},
			contains: []string{
				"ARG MAX_JOBS=1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateEngineDockerfile(tt.opts)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output should contain %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

func TestGenerateEngineDockerfile_Structure(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	required := []string{
		"FROM ubuntu:22.04 AS deps",
		"AS source",
		"AS generate",
		"AS builder",
		"AS runtime",
		"apt-get",
		"dnf",
		"build-essential",
		"ARG MAX_JOBS",
		"COPY . /engine",
		"WORKDIR /engine",
		"bash Setup.sh",
		"GenerateProjectFiles.sh",
		"make -j${MAX_JOBS} ShaderCompileWorker",
		"make -j${MAX_JOBS} UnrealEditor",
		"COPY --chown=ue:ue --from=builder",
		"useradd",
		"ENV UE_ROOT=/engine",
		`ENV PATH="/engine/Engine/Binaries/Linux:${PATH}"`,
		"mkdir -p /ddc",
		`CMD ["echo"`,
	}

	for _, elem := range required {
		if !strings.Contains(got, elem) {
			t.Errorf("output should contain %q\ngot:\n%s", elem, got)
		}
	}
}

func TestGenerateEngineDockerfile_MultiStage(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	// Must have five FROM statements (deps + source + generate + builder + runtime)
	fromCount := strings.Count(got, "\nFROM ")
	if strings.HasPrefix(got, "FROM ") {
		fromCount++
	}
	if fromCount != 5 {
		t.Errorf("multi-stage Dockerfile should have 5 FROM statements, got %d", fromCount)
	}

	// Each stage must be named
	assertContains(t, got, []string{"AS deps", "AS source", "AS generate", "AS builder", "AS runtime"})

	// Stages must chain correctly: source FROM deps, generate FROM source, builder FROM generate
	assertContains(t, got, []string{"FROM deps AS source", "FROM source AS generate", "FROM generate AS builder"})

	// Compile commands must be separate RUN statements for independent caching.
	// If UnrealEditor fails, ShaderCompileWorker shouldn't need recompilation.
	scwCount := strings.Count(got, "RUN make")
	if scwCount < 2 {
		t.Errorf("ShaderCompileWorker and UnrealEditor should be separate RUN commands, got %d make RUNs", scwCount)
	}

	// Intermediate dirs must NOT be stripped -- they save ~5 hours of recompilation
	// on each game build.
	if strings.Contains(got, "find") && strings.Contains(got, "Intermediate") {
		t.Error("builder stage should NOT strip Intermediate directories")
	}

	// Runtime stage must create the non-root build user
	if !strings.Contains(got, "useradd") || !strings.Contains(got, "ue") {
		t.Error("runtime stage should create a non-root 'ue' build user")
	}

	// Runtime stage should copy key engine directories from builder with --chown
	assertContains(t, got, []string{
		"COPY --chown=ue:ue --from=builder /engine/Engine/Binaries",
		"COPY --chown=ue:ue --from=builder /engine/Engine/Build",
		"COPY --chown=ue:ue --from=builder /engine/Engine/Config",
		"COPY --chown=ue:ue --from=builder /engine/Engine/Content",
		"COPY --chown=ue:ue --from=builder /engine/Engine/Plugins",
		"COPY --chown=ue:ue --from=builder /engine/Engine/Programs",
		"COPY --chown=ue:ue --from=builder /engine/Engine/Shaders",
		"COPY --chown=ue:ue --from=builder /engine/Engine/Source",
		"COPY --chown=ue:ue --from=builder /engine/Samples",
	})
}

func TestGenerateEngineDockerfile_AptPackages(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	// Verify all centralized package lists appear in the generated Dockerfile.
	// This ensures the Dockerfile stays in sync with the single source of truth in deps.go.
	for _, pkg := range aptBuildPackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("output should contain apt build package %q", pkg)
		}
	}
	for _, pkg := range AptRuntimePackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("output should contain apt runtime package %q", pkg)
		}
	}
}

func TestGenerateEngineDockerfile_DnfPackages(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	for _, pkg := range dnfBuildPackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("output should contain dnf build package %q", pkg)
		}
	}
	for _, pkg := range dnfRuntimePackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("output should contain dnf runtime package %q", pkg)
		}
	}
}

func TestGenerateEngineDockerfile_StartsWithComment(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})
	if !strings.HasPrefix(got, "#") {
		t.Errorf("Dockerfile should start with a stage comment, got: %q", got[:40])
	}
}

func TestGenerateEngineDockerfile_MacOSHost_Stage3Noop(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{MacOSHost: true})

	if strings.Contains(got, "bash Setup.sh") {
		t.Error("macOS host Dockerfile should not run Setup.sh in Stage 3")
	}
	if strings.Contains(got, "bash GenerateProjectFiles.sh") {
		t.Error("macOS host Dockerfile should not run GenerateProjectFiles.sh in Stage 3")
	}
	if !strings.Contains(got, "AS generate") {
		t.Error("macOS host Dockerfile must still have AS generate stage")
	}
	if !strings.Contains(got, "pre-flight") {
		t.Error("macOS host Dockerfile stage 3 should mention pre-flight")
	}
}

func TestGenerateEngineDockerfile_MacOSHost_LinuxTargets(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{MacOSHost: true})

	if !strings.Contains(got, "ShaderCompileWorker-Linux-Development") {
		t.Error("macOS host Dockerfile should use ShaderCompileWorker-Linux-Development target")
	}
	if !strings.Contains(got, "UnrealEditor-Linux-Development") {
		t.Error("macOS host Dockerfile should use UnrealEditor-Linux-Development target")
	}
	if strings.Contains(got, "make -j${MAX_JOBS} ShaderCompileWorker\n") {
		t.Error("macOS host Dockerfile must not use bare ShaderCompileWorker target")
	}
}

func TestGenerateEngineDockerfile_NonMacOSHost_UnchangedStage3(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{MacOSHost: false})

	if !strings.Contains(got, "bash Setup.sh") {
		t.Error("non-macOS Dockerfile should still run Setup.sh in Stage 3")
	}
	if !strings.Contains(got, "bash GenerateProjectFiles.sh") {
		t.Error("non-macOS Dockerfile should still run GenerateProjectFiles.sh in Stage 3")
	}
	if strings.Contains(got, "ShaderCompileWorker-Linux-Development") {
		t.Error("non-macOS Dockerfile should use bare make targets")
	}
}

func TestGenerateEngineDockerignore_ExcludesMacDotNet(t *testing.T) {
	got := GenerateEngineDockerignore()
	if !strings.Contains(got, "DotNet/mac-arm64") {
		t.Error("dockerignore should exclude mac-arm64 DotNet")
	}
	if !strings.Contains(got, "DotNet/mac-x64") {
		t.Error("dockerignore should exclude mac-x64 DotNet")
	}
}

// TestPreflightInstallCmd_Ubuntu22_04DotNetForUBT exercises engine build pre-flight
// on Ubuntu 22.04 container (critical for container engine builds / macOS pre-flights).
// Verifies the fix for 'System.Runtime.Numerics not found' by checking dotnet-sdk-8.0
// and the Microsoft repo setup in the generated preflight install command.
func TestPreflightInstallCmd_Ubuntu22_04DotNetForUBT(t *testing.T) {
	cmd := preflightInstallCmd("bash Setup.sh")
	if !strings.Contains(cmd, "packages-microsoft-prod.deb") {
		t.Error("preflight for Ubuntu 22.04 must setup Microsoft apt repo for dotnet-sdk-8.0")
	}
	if !strings.Contains(cmd, "dotnet-sdk-8.0") {
		t.Error("preflight must install dotnet-sdk-8.0 for UBT on Ubuntu 22.04 container")
	}
}
