package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadContainerIAMPath(t *testing.T) {
	for _, tt := range []struct {
		name, target, path string
		wantError          bool
	}{
		{"default path", "gamelift", "", false},
		{"root path", "gamelift", "/", false},
		{"unsupported path", "gamelift", "/project/", true},
		{"default target", "", "/project/", true},
		{"other target ignores container role", "ec2", "/project/", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ludus.yaml")
			data := "deploy:\n  target: '" + tt.target + "'\ngamelift:\n  iam:\n    path: '" + tt.path + "'\n"
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if (err != nil) != tt.wantError {
				t.Fatalf("Load error = %v, want error %v", err, tt.wantError)
			}
			if err != nil && !strings.Contains(err.Error(), "root-path role") {
				t.Fatalf("missing actionable error: %v", err)
			}
		})
	}
}
