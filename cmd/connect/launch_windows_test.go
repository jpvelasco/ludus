//go:build windows

package connect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchClient_Win64Success(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "client.bat")
	if err := os.WriteFile(binaryPath, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := launchClient(binaryPath, "Win64", t.TempDir(), "10.1.2.3:7777", "TestGame"); err != nil {
		t.Errorf("launchClient() error = %v, want nil", err)
	}
}

func TestLaunchClient_StartFailure(t *testing.T) {
	err := launchClient(filepath.Join(t.TempDir(), "missing.exe"), "Win64", t.TempDir(), "10.1.2.3:7777", "TestGame")
	if err == nil || !strings.Contains(err.Error(), "failed to launch client") {
		t.Errorf("launchClient() error = %v, want launch failure", err)
	}
}
