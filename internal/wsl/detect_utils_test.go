package wsl

import (
	"testing"
)

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
