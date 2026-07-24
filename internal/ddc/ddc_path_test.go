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
		{"", "zen", false}, // empty now defaults to zen (Zen is UE 5.4+ default)
		{"zen", "zen", false},
		{"local", "local", false}, // still valid, but deprecated
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

func TestDefaultPath_HomeUnset(t *testing.T) {
	// SSM Run Command / some CI contexts run with HOME stripped. DefaultPath must
	// still resolve via a fallback rather than hard-failing.
	if runtime.GOOS == "windows" {
		t.Skip("HOME-unset fallback is a *nix concern")
	}
	t.Setenv("HOME", "")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() with HOME unset should not error, got: %v", err)
	}
	if got == "" || !filepath.IsAbs(got) {
		t.Errorf("DefaultPath() = %q, want a non-empty absolute path", got)
	}
	if filepath.Base(got) != "ddc" {
		t.Errorf("DefaultPath() = %q, want it to end in .../ddc", got)
	}
}

func TestDefaultZenPath_HomeUnset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-unset fallback is a *nix concern")
	}
	t.Setenv("HOME", "")

	got, err := DefaultZenPath()
	if err != nil {
		t.Fatalf("DefaultZenPath() with HOME unset should not error, got: %v", err)
	}
	if got == "" || !filepath.IsAbs(got) {
		t.Errorf("DefaultZenPath() = %q, want a non-empty absolute path", got)
	}
	if filepath.Base(got) != "zen" {
		t.Errorf("DefaultZenPath() = %q, want it to end in .../zen", got)
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

func TestResolveZenPath(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	absolute := filepath.Join(home, "custom", "zen")
	tests := []struct {
		name     string
		override string
		want     string
		wantErr  bool
	}{
		{name: "default", want: filepath.Join(home, ".ludus", "zen")},
		{name: "absolute override", override: absolute, want: absolute},
		{name: "relative override", override: filepath.Join("relative", "zen"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveZenPath(tt.override)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveZenPath(%q) error = %v, wantErr %v", tt.override, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ResolveZenPath(%q) = %q, want %q", tt.override, got, tt.want)
			}
		})
	}
}

func TestDefaultZenPath(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	got, err := DefaultZenPath()
	if err != nil {
		t.Fatalf("DefaultZenPath() error = %v", err)
	}
	if want := filepath.Join(home, ".ludus", "zen"); got != want {
		t.Errorf("DefaultZenPath() = %q, want %q", got, want)
	}
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		return
	}
	t.Setenv("HOME", home)
}
