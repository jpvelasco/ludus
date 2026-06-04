//go:build darwin

package prereq

import (
	"testing"
)

func TestReadMemTotalGBDarwin_LiveSystem(t *testing.T) {
	gb, err := readMemTotalGBDarwin()
	if err != nil {
		t.Fatalf("readMemTotalGBDarwin() unexpected error: %v", err)
	}
	if gb == 0 {
		t.Error("expected non-zero memory on a real macOS machine")
	}
}

func TestCheckMemory_LiveSystem(t *testing.T) {
	c := &Checker{}
	result := c.checkMemory()
	if result.Name != "Memory" {
		t.Errorf("expected name 'Memory', got: %s", result.Name)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}
