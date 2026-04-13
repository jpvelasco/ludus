package wsl

import "testing"

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
