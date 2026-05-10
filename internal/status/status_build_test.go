package status

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

func TestCheckEngineSource_Empty(t *testing.T) {
	s := CheckEngineSource("")
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}
	if s.Detail != "not configured" {
		t.Errorf("expected detail 'not configured', got %q", s.Detail)
	}
}

func TestCheckEngineSource_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	s := CheckEngineSource(tmpDir)
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}

	expectedFile := "Setup.sh"
	if runtime.GOOS == "windows" {
		expectedFile = "Setup.bat"
	}
	expectedDetail := expectedFile + " not found"
	if s.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, s.Detail)
	}
}

func TestCheckEngineSource_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	setupFile := "Setup.sh"
	if runtime.GOOS == "windows" {
		setupFile = "Setup.bat"
	}

	setupPath := filepath.Join(tmpDir, setupFile)
	if err := os.WriteFile(setupPath, []byte("#!/bin/bash\necho setup"), 0644); err != nil {
		t.Fatal(err)
	}

	s := CheckEngineSource(tmpDir)
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}
	if s.Detail != tmpDir {
		t.Errorf("expected detail %q, got %q", tmpDir, s.Detail)
	}
}

func TestCheckEngineBuild_Empty(t *testing.T) {
	s := CheckEngineBuild("")
	if s.Status != "unknown" {
		t.Errorf("expected status 'unknown', got %q", s.Status)
	}
	if s.Detail != "source path not configured" {
		t.Errorf("expected detail 'source path not configured', got %q", s.Detail)
	}
}

func TestCheckEngineBuild_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	s := CheckEngineBuild(tmpDir)
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}

	expectedBinary := "UnrealEditor.exe"
	if runtime.GOOS != "windows" {
		expectedBinary = "UnrealEditor"
	}
	expectedDetail := expectedBinary + " not found"
	if s.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, s.Detail)
	}
}

func TestCheckEngineBuild_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	var editorPath string
	if runtime.GOOS == "windows" {
		editorPath = filepath.Join(tmpDir, "Engine", "Binaries", "Win64", "UnrealEditor.exe")
	} else {
		editorPath = filepath.Join(tmpDir, "Engine", "Binaries", "Linux", "UnrealEditor")
	}

	if err := os.MkdirAll(filepath.Dir(editorPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(editorPath, []byte("fake editor binary"), 0644); err != nil {
		t.Fatal(err)
	}

	s := CheckEngineBuild(tmpDir)
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}

	expectedBinary := "UnrealEditor.exe"
	if runtime.GOOS != "windows" {
		expectedBinary = "UnrealEditor"
	}
	expectedDetail := expectedBinary + " found"
	if s.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, s.Detail)
	}
}

func TestCheckServerBuild_Empty(t *testing.T) {
	s := CheckServerBuild("TestGame", "", "amd64")
	if s.Status != "unknown" {
		t.Errorf("expected status 'unknown', got %q", s.Status)
	}
	if s.Detail != "output directory unknown" {
		t.Errorf("expected detail 'output directory unknown', got %q", s.Detail)
	}
}

func TestCheckServerBuild_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	s := CheckServerBuild("TestGame", tmpDir, "amd64")
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}
	if s.Detail != "not built" {
		t.Errorf("expected detail 'not built', got %q", s.Detail)
	}
}

func TestCheckServerBuild_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create LinuxServer directory for amd64 arch
	serverDir := filepath.Join(tmpDir, config.ServerPlatformDir("amd64"))
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := CheckServerBuild("TestGame", tmpDir, "amd64")
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}
	if s.Detail != serverDir {
		t.Errorf("expected detail %q, got %q", serverDir, s.Detail)
	}
}
