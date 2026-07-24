package dockerbuild

import "testing"

func TestIsWSL2Backend(t *testing.T) {
	tests := []struct {
		backend string
		want    bool
	}{
		{backend: BackendWSL2, want: true},
		{backend: BackendDocker},
		{backend: BackendNative},
		{backend: ""},
		{backend: "WSL2"},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			if got := IsWSL2Backend(tt.backend); got != tt.want {
				t.Errorf("IsWSL2Backend(%q) = %v, want %v", tt.backend, got, tt.want)
			}
		})
	}
}

func TestContainerCLI_UsesPath(t *testing.T) {
	path := t.TempDir()
	executable := writeTestExecutable(t, path, BackendDocker)
	t.Setenv("PATH", path)
	if got := ContainerCLI(BackendDocker); got != executable {
		t.Errorf("ContainerCLI(docker) = %q, want %q", got, executable)
	}
}
