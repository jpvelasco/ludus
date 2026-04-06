package dockerbuild

import "testing"

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
