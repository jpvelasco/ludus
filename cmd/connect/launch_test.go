package connect

import (
	"strings"
	"testing"
)

func TestBuildLaunchArgs(t *testing.T) {
	args := buildLaunchArgs("192.168.1.100:7777")

	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}

	// First arg must be the server address as a travel URL
	if args[0] != "192.168.1.100:7777" {
		t.Errorf("first arg should be server address, got %q", args[0])
	}

	if args[1] != "-game" {
		t.Errorf("second arg should be -game, got %q", args[1])
	}

	if args[2] != "-log" {
		t.Errorf("third arg should be -log, got %q", args[2])
	}

	// Must not contain -connect flag
	for _, arg := range args {
		if strings.Contains(arg, "-connect") {
			t.Errorf("args should not contain -connect flag, got %q", arg)
		}
	}
}

func TestBuildLaunchArgs_PortVariations(t *testing.T) {
	tests := []struct {
		addr string
	}{
		{"10.0.0.1:7777"},
		{"44.249.62.213:4192"},
		{"127.0.0.1:30000"},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			args := buildLaunchArgs(tt.addr)
			if args[0] != tt.addr {
				t.Errorf("first arg = %q, want %q", args[0], tt.addr)
			}
		})
	}
}
