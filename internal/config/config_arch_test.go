package config

import "testing"

func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "amd64"},
		{"amd64", "amd64"},
		{"x86_64", "amd64"},
		{"arm64", "arm64"},
		{"aarch64", "arm64"},
		{"AMD64", "amd64"},
		{"ARM64", "arm64"},
		{"AArch64", "arm64"},
		{"  arm64  ", "arm64"},
		{"mips64", "amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeArch(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeArch(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestServerPlatformDir(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "LinuxServer"},
		{"x86_64", "LinuxServer"},
		{"", "LinuxServer"},
		{"arm64", "LinuxArm64Server"},
		{"aarch64", "LinuxArm64Server"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := ServerPlatformDir(tt.arch)
			if got != tt.want {
				t.Errorf("ServerPlatformDir(%q) = %q, want %q", tt.arch, got, tt.want)
			}
		})
	}
}

func TestBinariesPlatformDir(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "Linux"},
		{"", "Linux"},
		{"arm64", "LinuxArm64"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := BinariesPlatformDir(tt.arch)
			if got != tt.want {
				t.Errorf("BinariesPlatformDir(%q) = %q, want %q", tt.arch, got, tt.want)
			}
		})
	}
}

func TestUEPlatformName(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "Linux"},
		{"", "Linux"},
		{"arm64", "Linux"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := UEPlatformName(tt.arch)
			if got != tt.want {
				t.Errorf("UEPlatformName(%q) = %q, want %q", tt.arch, got, tt.want)
			}
		})
	}
}

func TestUEServerPlatformName(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "Linux"},
		{"", "Linux"},
		{"arm64", "Linux.LinuxArm64"},
		{"aarch64", "Linux.LinuxArm64"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			if got := UEServerPlatformName(tt.arch); got != tt.want {
				t.Errorf("UEServerPlatformName(%q) = %q, want %q", tt.arch, got, tt.want)
			}
		})
	}
}
