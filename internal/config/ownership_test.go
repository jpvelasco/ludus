package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsReservedOwnershipKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ludus.yaml")
	if err := os.WriteFile(path, []byte("gamelift:\n  ownershipTagKey: Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "cannot replace") {
		t.Fatalf("reserved ownership key accepted: %v", err)
	}
}

func TestOwnershipTagKey(t *testing.T) {
	for _, tt := range []struct {
		key string
		bad bool
	}{
		{"", false}, {"ludus:fleet-name", false}, {"Deployment", false},
		{"project", true}, {"MANAGEDBY", true}, {"ludus:target", true},
		{"AWS:reserved", true}, {"  ", true}, {strings.Repeat("x", 129), true},
	} {
		t.Run(tt.key, func(t *testing.T) {
			if err := ValidateOwnershipTagKey(tt.key); (err != nil) != tt.bad {
				t.Fatalf("validation = %v, want failure %v", err, tt.bad)
			}
		})
	}
	if ResolvedOwnershipTagKey("") != DefaultFleetOwnershipTagKey {
		t.Fatal("default ownership key changed")
	}
}
