package dockerbuild

import (
	"strings"
	"testing"
)

func TestGenerateEngineDockerfile(t *testing.T) {
	tests := []struct {
		name     string
		opts     DockerfileOptions
		contains []string
	}{
		{
			name: "defaults use ubuntu and 4 jobs",
			opts: DockerfileOptions{},
			contains: []string{
				"FROM ubuntu:22.04",
				"ARG MAX_JOBS=4",
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
			name: "zero max jobs defaults to 4",
			opts: DockerfileOptions{MaxJobs: 0},
			contains: []string{
				"ARG MAX_JOBS=4",
			},
		},
		{
			name: "negative max jobs defaults to 4",
			opts: DockerfileOptions{MaxJobs: -10},
			contains: []string{
				"ARG MAX_JOBS=4",
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

func TestGeneratePrebuiltEngineDockerfile(t *testing.T) {
	tests := []struct {
		name     string
		opts     DockerfileOptions
		contains []string
	}{
		{
			name: "defaults use ubuntu",
			opts: DockerfileOptions{},
			contains: []string{
				"FROM ubuntu:22.04 AS deps",
			},
		},
		{
			name: "custom base image",
			opts: DockerfileOptions{BaseImage: "amazonlinux:2023"},
			contains: []string{
				"FROM amazonlinux:2023 AS deps",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GeneratePrebuiltEngineDockerfile(tt.opts)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output should contain %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

func TestGeneratePrebuiltEngineDockerfile_Structure(t *testing.T) {
	got := GeneratePrebuiltEngineDockerfile(DockerfileOptions{})

	// Must be a 2-stage Dockerfile (deps + runtime)
	fromCount := strings.Count(got, "\nFROM ")
	if strings.HasPrefix(got, "FROM ") {
		fromCount++
	}
	// The header comment precedes FROM, so count FROM lines
	if fromCount != 2 {
		t.Errorf("prebuilt Dockerfile should have 2 FROM statements, got %d", fromCount)
	}

	required := []string{
		"AS deps",
		"AS runtime",
		"FROM deps AS runtime",
		"apt-get",
		"dnf",
		"build-essential",
		"ENV UE_ROOT=/engine",
		`ENV PATH="/engine/Engine/Binaries/Linux:${PATH}"`,
		"mkdir -p /ddc",
		"COPY --chown=ue:ue Engine/Binaries",
		"COPY --chown=ue:ue Engine/Build",
		"COPY --chown=ue:ue Engine/Config",
		"COPY --chown=ue:ue Engine/Content",
		"COPY --chown=ue:ue Engine/Plugins",
		"COPY --chown=ue:ue Engine/Programs",
		"COPY --chown=ue:ue Engine/Shaders",
		"COPY --chown=ue:ue Engine/Source",
		"COPY --chown=ue:ue Samples",
		"COPY --chown=ue:ue Setup.sh",
		"COPY --chown=ue:ue GenerateProjectFiles.sh",
		`CMD ["echo"`,
	}

	for _, elem := range required {
		if !strings.Contains(got, elem) {
			t.Errorf("output should contain %q\ngot:\n%s", elem, got)
		}
	}
}

func TestGeneratePrebuiltEngineDockerfile_OwnershipFix(t *testing.T) {
	got := GeneratePrebuiltEngineDockerfile(DockerfileOptions{})

	// The Dockerfile must fix ownership on directories game builds write to.
	// COPY --chown is silently ignored by Podman on NTFS/virtiofs build contexts,
	// leaving all files root-owned. The targeted RUN chown fixes only writable
	// directories to avoid a full recursive chown on the 100+ GB engine tree.
	chownTargets := []string{
		"chown -R ue:ue /engine/Engine/Binaries/Linux",
		"*/Binaries/Linux",
		"*/Build/Scripts/obj",
	}
	for _, target := range chownTargets {
		if !strings.Contains(got, target) {
			t.Errorf("prebuilt Dockerfile should fix ownership on %q\ngot:\n%s", target, got)
		}
	}

	// The ownership fix must appear AFTER all COPY steps and BEFORE CMD.
	lastCopy := strings.LastIndex(got, "COPY --chown=ue:ue")
	chownFix := strings.Index(got, "chown -R ue:ue /engine/Engine/Binaries/Linux")
	cmdLine := strings.Index(got, `CMD ["echo"`)
	if lastCopy >= chownFix {
		t.Error("ownership fix RUN must appear after all COPY steps")
	}
	if chownFix >= cmdLine {
		t.Error("ownership fix RUN must appear before CMD")
	}
}

func TestGeneratePrebuiltEngineDockerfile_NoCompileStages(t *testing.T) {
	got := GeneratePrebuiltEngineDockerfile(DockerfileOptions{})

	// Must NOT have compile-related stages or commands
	forbidden := []string{
		"AS source",
		"AS generate",
		"AS builder",
		"make -j",
		"COPY --from=builder",
	}

	for _, elem := range forbidden {
		if strings.Contains(got, elem) {
			t.Errorf("prebuilt Dockerfile must NOT contain %q (no compilation)", elem)
		}
	}
}

func TestGeneratePrebuiltEngineDockerignore_Prebuilt(t *testing.T) {
	got := GeneratePrebuiltEngineDockerignore()

	contains := []string{
		".git",
		"*.md",
		"**/Intermediate/",
		"**/Saved/",
		"**/Binaries/Win64/",
		"**/Binaries/Mac/",
		"**/*.pdb",
		"**/*.dSYM",
		"**/PackagedServer/",
		"**/PackagedClient/",
		"FeaturePacks/",
	}

	for _, want := range contains {
		if !strings.Contains(got, want) {
			t.Errorf("prebuilt dockerignore should contain %q\ngot:\n%s", want, got)
		}
	}
}

func TestGenerateEngineDockerignore(t *testing.T) {
	tests := []struct {
		name     string
		contains []string
	}{
		{
			name:     "version control patterns",
			contains: []string{".git", ".github", ".gitignore", ".gitattributes"},
		},
		{
			name:     "documentation patterns",
			contains: []string{"*.md", "LICENSE"},
		},
		{
			name:     "IDE patterns",
			contains: []string{".vscode", ".idea", ".vs", "*.sln", "*.suo", "*.user", "*.xcodeproj", "*.xcworkspace"},
		},
		{
			name:     "host build artifacts",
			contains: []string{"**/Intermediate/", "**/Saved/", "Engine/DerivedDataCache/"},
		},
		{
			name:     "host platform binaries",
			contains: []string{"**/Binaries/Win64/", "**/Binaries/Mac/"},
		},
		{
			name:     "host debug symbols",
			contains: []string{"**/*.pdb", "**/*.dSYM"},
		},
		{
			name:     "previous build outputs",
			contains: []string{"**/PackagedServer/", "**/PackagedClient/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateEngineDockerignore()
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output should contain %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

func TestGenerateEngineDockerignore_HasMultipleLines(t *testing.T) {
	got := GenerateEngineDockerignore()
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 5 {
		t.Errorf("should have at least 5 lines, got %d", len(lines))
	}
}

// Regression test: **/DerivedDataCache/ was too broad and excluded the UE5
// source module at Engine/Source/Developer/DerivedDataCache/, breaking
// ShaderCompileWorker compilation. Only the cache data dir should be excluded.
func TestGenerateEngineDockerignore_PreservesSourceModules(t *testing.T) {
	got := GenerateEngineDockerignore()

	// Check active (non-comment) lines only — comments may reference the pattern.
	for _, line := range strings.Split(got, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "**/DerivedDataCache/" {
			t.Fatal("dockerignore must not use **/DerivedDataCache/ as an active rule — " +
				"it excludes Engine/Source/Developer/DerivedDataCache/ which is a required source module")
		}
	}

	// Must contain the scoped exclusion for the cache data directory only
	if !strings.Contains(got, "Engine/DerivedDataCache/") {
		t.Error("dockerignore should exclude Engine/DerivedDataCache/ (cache data)")
	}
}

func TestGenerateEngineDockerignore_HasComments(t *testing.T) {
	got := GenerateEngineDockerignore()
	hasComment := false
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			hasComment = true
			break
		}
	}
	if !hasComment {
		t.Error("dockerignore should contain at least one comment line")
	}
}
