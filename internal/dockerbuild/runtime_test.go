package dockerbuild

import (
	"fmt"
	"strings"
	"testing"
)

func TestIsContainerBackend(t *testing.T) {
	tests := []struct {
		backend string
		want    bool
	}{
		{"docker", true},
		{"podman", true},
		{"native", false},
		{"", false},
		{"kubernetes", false},
	}
	for _, tt := range tests {
		if got := IsContainerBackend(tt.backend); got != tt.want {
			t.Errorf("IsContainerBackend(%q) = %v, want %v", tt.backend, got, tt.want)
		}
	}
}

func TestContainerCLI(t *testing.T) {
	tests := []struct {
		backend string
		want    string
	}{
		{"docker", "docker"},
		{"podman", "podman"},
		{"native", "docker"},
		{"", "docker"},
	}
	for _, tt := range tests {
		if got := ContainerCLI(tt.backend); got != tt.want {
			t.Errorf("ContainerCLI(%q) = %q, want %q", tt.backend, got, tt.want)
		}
	}
}

func TestIsContainerdLeaseError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"lease xyz not found", true},
		{"failed to solve: exporting to image format", true},
		{"build completed successfully", false},
		{"connection refused", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isContainerdLeaseError(tt.msg); got != tt.want {
			t.Errorf("isContainerdLeaseError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestWrapBuildError_DockerLeaseTimeout(t *testing.T) {
	err := fmt.Errorf("lease abc123 not found")
	wrapped := wrapBuildError("docker", err)
	msg := wrapped.Error()

	if !strings.Contains(msg, "containerd lease timeout") {
		t.Error("should mention containerd lease timeout")
	}
	if !strings.Contains(msg, "podman") {
		t.Error("should recommend podman")
	}
	if !strings.Contains(msg, "--skip-compile") {
		t.Error("should recommend --skip-compile")
	}
}

func TestWrapBuildError_PodmanNoSpecialMessage(t *testing.T) {
	err := fmt.Errorf("lease abc123 not found")
	wrapped := wrapBuildError("podman", err)
	msg := wrapped.Error()

	if strings.Contains(msg, "containerd") {
		t.Error("podman errors should not mention containerd")
	}
	if !strings.Contains(msg, "podman build failed") {
		t.Errorf("should say podman build failed, got: %s", msg)
	}
}

func TestWrapBuildError_GenericError(t *testing.T) {
	err := fmt.Errorf("permission denied")
	wrapped := wrapBuildError("docker", err)
	msg := wrapped.Error()

	if strings.Contains(msg, "containerd") {
		t.Error("generic errors should not trigger containerd advice")
	}
	if !strings.Contains(msg, "docker build failed") {
		t.Errorf("should say docker build failed, got: %s", msg)
	}
}
