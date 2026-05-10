package ddc

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateDDCMode(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", "local", false},
		{"local", "local", false},
		{"none", "none", false},
		{"shared", "", true},
		{"LOCAL", "", true},
	}
	for _, tt := range tests {
		got, err := ValidateDDCMode(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateDDCMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ValidateDDCMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error: %v", err)
	}

	want := filepath.Join(home, ".ludus", "ddc")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestResolvePath_Override(t *testing.T) {
	path := "/custom/ddc"
	if runtime.GOOS == "windows" {
		path = `C:\custom\ddc`
	}
	got, err := ResolvePath(path)
	if err != nil {
		t.Fatalf("ResolvePath() error: %v", err)
	}
	if got != path {
		t.Errorf("ResolvePath(%q) = %q, want %q", path, got, path)
	}
}

func TestResolvePath_RelativeErrors(t *testing.T) {
	_, err := ResolvePath("relative/ddc")
	if err == nil {
		t.Error("ResolvePath() should error for relative path")
	}
}

func TestResolvePath_Default(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	got, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath() error: %v", err)
	}

	want := filepath.Join(home, ".ludus", "ddc")
	if got != want {
		t.Errorf("ResolvePath(%q) = %q, want %q", "", got, want)
	}
}
