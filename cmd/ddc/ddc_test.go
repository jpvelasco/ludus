package ddc

import "testing"

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"one byte", 1, "1 B"},
		{"below KB", 1023, "1023 B"},
		{"exact KB", 1024, "1.0 KB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"below MB", 1024*1024 - 1, "1024.0 KB"},
		{"exact MB", 1024 * 1024, "1.0 MB"},
		{"500 MB", 500 * 1024 * 1024, "500.0 MB"},
		{"below GB", 1024*1024*1024 - 1, "1024.0 MB"},
		{"exact GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"2.5 GB", 2.5 * 1024 * 1024 * 1024, "2.5 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
