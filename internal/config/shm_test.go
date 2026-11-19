package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEngineShmSize(t *testing.T) {
	for _, tt := range []struct {
		value     string
		wantError bool
	}{
		{"", false},
		{"2g", false},
		{"1024m", false},
		{"2GiB", false},
		{"1.5g", false},
		{"536870912", false},
		{"two gigs", true},
		{"-1g", true},
		{"2x", true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ludus.yaml")
			if err := os.WriteFile(path, []byte("engine:\n  shmSize: '"+tt.value+"'\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if (err != nil) != tt.wantError {
				t.Fatalf("Load error = %v, want error %v", err, tt.wantError)
			}
			if err != nil && !strings.Contains(err.Error(), "engine.shmSize") {
				t.Fatalf("error %q should name engine.shmSize", err)
			}
			if err == nil && cfg.Engine.ShmSize != tt.value {
				t.Fatalf("ShmSize = %q, want %q", cfg.Engine.ShmSize, tt.value)
			}
		})
	}
}
