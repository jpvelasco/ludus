package dockerbuild

import (
	"strings"
	"testing"
)

// assertContains fails the test for each pattern not found in text.
func assertContains(t *testing.T, text string, patterns []string) {
	t.Helper()
	for _, p := range patterns {
		if !strings.Contains(text, p) {
			t.Errorf("output should contain %q", p)
		}
	}
}

// checkDDCArgs calls ddcArgs on b and verifies the result against wantErr/wantNoArgs/wantArgs.
func checkDDCArgs(t *testing.T, b *DockerGameBuilder, wantErr string, wantNoArgs bool, wantArgs []string) {
	t.Helper()
	args, err := b.ddcArgs()
	if wantErr != "" {
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), wantErr) {
			t.Errorf("error should contain %q, got: %v", wantErr, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wantNoArgs {
		if len(args) != 0 {
			t.Errorf("expected no args, got %v", args)
		}
		return
	}
	assertContains(t, strings.Join(args, " "), wantArgs)
}
