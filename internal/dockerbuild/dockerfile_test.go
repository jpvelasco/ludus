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
		"FROM ubuntu:22.04 AS builder",
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
		"COPY --from=builder",
		"ENV UE_ROOT=/engine",
		`ENV PATH="/engine/Engine/Binaries/Linux:${PATH}"`,
		`CMD ["bash"]`,
	}

	for _, elem := range required {
		if !strings.Contains(got, elem) {
			t.Errorf("output should contain %q\ngot:\n%s", elem, got)
		}
	}
}

func TestGenerateEngineDockerfile_MultiStage(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	// Must have two FROM statements (builder + runtime)
	fromCount := strings.Count(got, "\nFROM ")
	// Account for the first FROM at start of string
	if strings.HasPrefix(got, "FROM ") {
		fromCount++
	}
	if fromCount < 2 {
		t.Errorf("multi-stage Dockerfile should have at least 2 FROM statements, got %d", fromCount)
	}

	// Must strip Intermediate dirs in the builder stage
	if !strings.Contains(got, "Intermediate") {
		t.Error("builder stage should strip Intermediate directories")
	}

	// Runtime stage should copy key engine directories from builder
	runtimeCopies := []string{
		"COPY --from=builder /engine/Engine/Binaries",
		"COPY --from=builder /engine/Engine/Build",
		"COPY --from=builder /engine/Engine/Config",
		"COPY --from=builder /engine/Engine/Content",
		"COPY --from=builder /engine/Engine/Plugins",
		"COPY --from=builder /engine/Engine/Programs",
		"COPY --from=builder /engine/Engine/Shaders",
		"COPY --from=builder /engine/Engine/Source",
		"COPY --from=builder /engine/Samples",
	}
	for _, want := range runtimeCopies {
		if !strings.Contains(got, want) {
			t.Errorf("runtime stage should contain %q", want)
		}
	}
}

func TestGenerateEngineDockerfile_AptPackages(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	aptPackages := []string{
		"build-essential",
		"git",
		"cmake",
		"python3",
		"curl",
		"xdg-user-dirs",
		"shared-mime-info",
		"libfontconfig1",
		"libfreetype6",
		"libc6-dev",
	}

	for _, pkg := range aptPackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("output should contain apt package %q", pkg)
		}
	}
}

func TestGenerateEngineDockerfile_DnfPackages(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	dnfPackages := []string{
		"gcc",
		"gcc-c++",
		"cmake",
		"python3",
		"curl",
		"fontconfig-devel",
		"freetype-devel",
		"glibc-devel",
	}

	for _, pkg := range dnfPackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("output should contain dnf package %q", pkg)
		}
	}
}

func TestGenerateEngineDockerfile_StartsWithComment(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})
	if !strings.HasPrefix(got, "#") {
		t.Errorf("Dockerfile should start with a stage comment, got: %q", got[:40])
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
