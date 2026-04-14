package wsl

import (
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/dockerbuild"
)

func TestParseDistroList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []DistroInfo
	}{
		{
			"typical output",
			"  NAME            STATE           VERSION\n* Ubuntu          Running         2\n  Debian          Stopped         2\n",
			[]DistroInfo{
				{Name: "Ubuntu", Version: 2, Running: true, Default: true},
				{Name: "Debian", Version: 2, Running: false, Default: false},
			},
		},
		{
			"single distro",
			"  NAME            STATE           VERSION\n* Ubuntu-24.04    Running         2\n",
			[]DistroInfo{
				{Name: "Ubuntu-24.04", Version: 2, Running: true, Default: true},
			},
		},
		{
			"mixed WSL versions",
			"  NAME            STATE           VERSION\n* Ubuntu          Running         2\n  Legacy          Stopped         1\n",
			[]DistroInfo{
				{Name: "Ubuntu", Version: 2, Running: true, Default: true},
				{Name: "Legacy", Version: 1, Running: false, Default: false},
			},
		},
		{
			"with NUL bytes (UTF-16LE artifact)",
			"\x00 \x00 \x00N\x00A\x00M\x00E\x00",
			nil,
		},
		{
			"empty output",
			"",
			nil,
		},
		{
			"header only",
			"  NAME            STATE           VERSION\n",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDistroList(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("parseDistroList() returned %d distros, want %d\ngot: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("distro[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPickDistro(t *testing.T) {
	info := &Info{
		Available: true,
		Distros: []DistroInfo{
			{Name: "Ubuntu", Version: 2, Running: true, Default: true},
			{Name: "Debian", Version: 2, Running: false, Default: false},
			{Name: "Legacy", Version: 1, Running: true, Default: false},
		},
	}

	tests := []struct {
		name     string
		override string
		want     string
		wantErr  bool
	}{
		{"picks first running WSL2", "", "Ubuntu", false},
		{"override exact match", "Debian", "Debian", false},
		{"override case insensitive", "ubuntu", "Ubuntu", false},
		{"override WSL1 errors", "Legacy", "", true},
		{"override not found", "Nonexistent", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PickDistro(info, tt.override)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PickDistro(%q) error = %v, wantErr %v", tt.override, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("PickDistro(%q) = %q, want %q", tt.override, got, tt.want)
			}
		})
	}
}

func TestPickDistroNoRunning(t *testing.T) {
	info := &Info{
		Available: true,
		Distros: []DistroInfo{
			{Name: "Ubuntu", Version: 2, Running: false, Default: true},
		},
	}
	got, err := PickDistro(info, "")
	if err != nil {
		t.Fatalf("PickDistro() error = %v", err)
	}
	if got != "Ubuntu" {
		t.Errorf("PickDistro() = %q, want %q", got, "Ubuntu")
	}
}

func TestPickDistroEmpty(t *testing.T) {
	info := &Info{Available: false, Distros: nil}
	_, err := PickDistro(info, "")
	if err == nil {
		t.Error("PickDistro() expected error for empty distros")
	}
}

func TestParseDiskFreeGB(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    float64
		wantErr bool
	}{
		{"typical df output", " Avail\n  250G\n", 250, false},
		{"no G suffix", " Avail\n  100\n", 100, false},
		{"empty", "", 0, true},
		{"header only", "Avail\n", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDiskFreeGB(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDiskFreeGB() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseDiskFreeGB() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestCleanWSLOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"with NULs", "h\x00e\x00l\x00l\x00o", "hello"},
		{"with BOM", "\xef\xbb\xbfhello", "hello"},
		{"with CR", "hello\r\nworld", "hello\nworld"},
		{"combined", "\xef\xbb\xbfh\x00e\x00l\x00l\x00o\r\n", "hello\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanWSLOutput(tt.input)
			if got != tt.want {
				t.Errorf("cleanWSLOutput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInstallDepsScriptIncludesRuntimePackages(t *testing.T) {
	script := installDepsScript()

	// Every runtime package from AptRuntimePackages must appear in the script.
	// This is the single source of truth for UE5 runtime deps (identified via
	// ldd audit of UnrealEditor-Cmd).
	for _, pkg := range dockerbuild.AptRuntimePackages {
		if !strings.Contains(script, pkg) {
			t.Errorf("installDepsScript() missing runtime package %q", pkg)
		}
	}

	// Build deps must still be present.
	for _, pkg := range []string{"build-essential", "cmake", "python3", "rsync"} {
		if !strings.Contains(script, pkg) {
			t.Errorf("installDepsScript() missing build package %q", pkg)
		}
	}
}

func TestInstallRuntimeDepsScript(t *testing.T) {
	script := installRuntimeDepsScript()

	for _, pkg := range dockerbuild.AptRuntimePackages {
		if !strings.Contains(script, pkg) {
			t.Errorf("installRuntimeDepsScript() missing package %q", pkg)
		}
	}

	// Must be non-interactive.
	if !strings.Contains(script, "DEBIAN_FRONTEND=noninteractive") {
		t.Error("runtime deps script must set DEBIAN_FRONTEND=noninteractive")
	}
	if !strings.Contains(script, "-y") {
		t.Error("runtime deps script must use -y for non-interactive install")
	}
}

func TestInstallDepsScriptIsIdempotent(t *testing.T) {
	script := installDepsScript()

	// apt-get install -y is idempotent: re-installing already-installed
	// packages is a no-op. Verify the script uses -y.
	if !strings.Contains(script, "install -y") {
		t.Error("installDepsScript() must use 'install -y' for idempotent operation")
	}
}

func TestInstallDepsScriptHandlesT64Packages(t *testing.T) {
	script := installDepsScript()

	// The script must handle Ubuntu 24.04 t64 package renames.
	for oldName, t64Name := range dockerbuild.AptRuntimeT64Packages {
		if !strings.Contains(script, oldName) && !strings.Contains(script, t64Name) {
			t.Errorf("script missing both %q and t64 variant %q", oldName, t64Name)
		}
		if !strings.Contains(script, t64Name) {
			t.Errorf("script missing t64 fallback %q for %q", t64Name, oldName)
		}
	}
}

func TestInstallScriptsRunAsRoot(t *testing.T) {
	// Scripts run via wsl.exe -u root, so they must NOT use sudo.
	// Using sudo under -u root is redundant and may fail if sudo is not installed.
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"installDepsScript", installDepsScript()},
		{"installRuntimeDepsScript", installRuntimeDepsScript()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(tc.script, "sudo ") {
				t.Errorf("%s should not use sudo (runs as root via wsl.exe -u root)", tc.name)
			}
		})
	}
}
