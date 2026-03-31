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
		"FROM",
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
	}

	for _, elem := range required {
		if !strings.Contains(got, elem) {
			t.Errorf("output should contain %q\ngot:\n%s", elem, got)
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

func TestGenerateEngineDockerfile_StartsWithFROM(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})
	if !strings.HasPrefix(got, "FROM ") {
		t.Errorf("Dockerfile should start with FROM, got: %q", got[:40])
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
			contains: []string{".vscode", ".idea", "*.sln", "*.xcodeproj", "*.xcworkspace"},
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
