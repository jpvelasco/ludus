package dockerbuild

import (
	"strings"
	"testing"
)

func TestGenerateEngineDockerfile_Defaults(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	// Verify default base image
	if !strings.Contains(got, "FROM ubuntu:22.04") {
		t.Errorf("GenerateEngineDockerfile() with defaults should contain 'FROM ubuntu:22.04', got: %s", got)
	}

	// Verify default max jobs
	if !strings.Contains(got, "ARG MAX_JOBS=4") {
		t.Errorf("GenerateEngineDockerfile() with defaults should contain 'ARG MAX_JOBS=4', got: %s", got)
	}
}

func TestGenerateEngineDockerfile_Custom(t *testing.T) {
	opts := DockerfileOptions{
		MaxJobs:   8,
		BaseImage: "amazonlinux:2023",
	}

	got := GenerateEngineDockerfile(opts)

	// Verify custom base image
	if !strings.Contains(got, "FROM amazonlinux:2023") {
		t.Errorf("GenerateEngineDockerfile() should contain 'FROM amazonlinux:2023', got: %s", got)
	}

	// Verify custom max jobs
	if !strings.Contains(got, "ARG MAX_JOBS=8") {
		t.Errorf("GenerateEngineDockerfile() should contain 'ARG MAX_JOBS=8', got: %s", got)
	}
}

func TestGenerateEngineDockerfile_ZeroMaxJobs(t *testing.T) {
	opts := DockerfileOptions{
		MaxJobs: 0,
	}

	got := GenerateEngineDockerfile(opts)

	// Zero MaxJobs should default to 4
	if !strings.Contains(got, "ARG MAX_JOBS=4") {
		t.Errorf("GenerateEngineDockerfile() with MaxJobs=0 should default to 4, got: %s", got)
	}
}

func TestGenerateEngineDockerfile_NegativeMaxJobs(t *testing.T) {
	opts := DockerfileOptions{
		MaxJobs: -5,
	}

	got := GenerateEngineDockerfile(opts)

	// Negative MaxJobs should default to 4
	if !strings.Contains(got, "ARG MAX_JOBS=4") {
		t.Errorf("GenerateEngineDockerfile() with MaxJobs=-5 should default to 4, got: %s", got)
	}
}

func TestGenerateEngineDockerfile_Structure(t *testing.T) {
	got := GenerateEngineDockerfile(DockerfileOptions{})

	// Verify key structural elements are present
	requiredElements := []string{
		"FROM",
		"RUN",
		"ARG MAX_JOBS",
		"COPY . /engine",
		"WORKDIR /engine",
		"bash Setup.sh",
		"GenerateProjectFiles.sh",
		"make",
		"ShaderCompileWorker",
		"UnrealEditor",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(got, elem) {
			t.Errorf("GenerateEngineDockerfile() should contain %q, got: %s", elem, got)
		}
	}
}

func TestGenerateEngineDockerignore(t *testing.T) {
	got := GenerateEngineDockerignore()

	// Verify key patterns are present
	requiredPatterns := []string{
		".git",
		".github",
		"*.md",
	}

	for _, pattern := range requiredPatterns {
		if !strings.Contains(got, pattern) {
			t.Errorf("GenerateEngineDockerignore() should contain %q, got: %s", pattern, got)
		}
	}
}

func TestGenerateEngineDockerignore_NotEmpty(t *testing.T) {
	got := GenerateEngineDockerignore()

	if len(got) == 0 {
		t.Error("GenerateEngineDockerignore() should return non-empty string")
	}

	// Should have multiple lines
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 5 {
		t.Errorf("GenerateEngineDockerignore() should have multiple lines, got %d lines", len(lines))
	}
}
