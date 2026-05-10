package wsl

import (
	"testing"
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
