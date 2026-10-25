package globals

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestResolveConfigFile(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		profile    string
		want       string
	}{
		{"explicit selection wins over profile", "custom.yaml", "ue57", "custom.yaml"},
		{"named profile", "", "ue57", "ludus-ue57.yaml"},
		{"default", "", "", "ludus.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origConfig, origProfile := ConfigFile, Profile
			t.Cleanup(func() { ConfigFile, Profile = origConfig, origProfile })

			ConfigFile = tt.configFile
			Profile = tt.profile

			if got := ResolveConfigFile(); got != tt.want {
				t.Errorf("ResolveConfigFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewConfigDocument(t *testing.T) {
	t.Run("missing file yields empty document", testNewConfigDocumentMissing)
	t.Run("existing file seeds the document", testNewConfigDocumentSeeded)
	t.Run("malformed file returns a blank document with the error", testNewConfigDocumentMalformed)
}

func testNewConfigDocumentMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")

	v, exists, err := NewConfigDocument(path)
	if err != nil {
		t.Fatalf("NewConfigDocument() error: %v", err)
	}
	if exists {
		t.Error("exists = true, want false for a missing file")
	}
	if v == nil {
		t.Fatal("document = nil, want a usable empty document")
	}
}

func testNewConfigDocumentSeeded(t *testing.T) {
	path := writeConfigFile(t, "engine:\n  version: 5.7.3\n")

	v, exists, err := NewConfigDocument(path)
	if err != nil {
		t.Fatalf("NewConfigDocument() error: %v", err)
	}
	if !exists {
		t.Error("exists = false, want true for a present file")
	}
	if got := v.GetString("engine.version"); got != "5.7.3" {
		t.Errorf("engine.version = %q, want the value from the file", got)
	}
}

// testNewConfigDocumentMalformed pins the blank-document-alongside-error
// contract: the confirmed-overwrite path in setup writes a fresh document
// from the returned instance.
func testNewConfigDocumentMalformed(t *testing.T) {
	path := writeConfigFile(t, "engine:\n  version: [5.8\n")

	v, exists, err := NewConfigDocument(path)
	if err == nil {
		t.Fatal("NewConfigDocument() error = nil, want the parse error")
	}
	if exists {
		t.Error("exists = true, want false for an unreadable file")
	}

	v.Set("ddc.mode", "none")
	if err := SaveConfigDocument(v, path); err != nil {
		t.Fatalf("SaveConfigDocument() after failed read: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mode: none") {
		t.Errorf("rewritten config = %q, want the fresh document", data)
	}
}

func TestSaveConfigDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ludus.yaml")

	v, _, err := NewConfigDocument(path)
	if err != nil {
		t.Fatalf("NewConfigDocument() error: %v", err)
	}
	v.Set("ddc.mode", "zen")

	if err := SaveConfigDocument(v, path); err != nil {
		t.Fatalf("SaveConfigDocument() error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mode: zen") {
		t.Errorf("saved config = %q, want the ddc mode written", data)
	}
}

func TestSaveConfigDocument_Error(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.Set("ddc.mode", "zen")

	unwritable := filepath.Join(t.TempDir(), "missing", "ludus.yaml")
	err := SaveConfigDocument(v, unwritable)
	if err == nil || !strings.Contains(err.Error(), "writing "+unwritable) {
		t.Errorf("SaveConfigDocument() error = %v, want a wrapped write failure", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("SaveConfigDocument() error = %v, want it to wrap the cause", err)
	}
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ludus.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
