package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPTransportSelection(t *testing.T) {
	for _, tt := range []struct {
		name, transport, listen, want string
		explicit, remote, bad         bool
	}{
		{"default", "stdio", "", "stdio", false, false, false},
		{"listen implies http", "stdio", "127.0.0.1:8080", "http", false, false, false},
		{"explicit http", "http", "", "http", true, false, false},
		{"stdio conflicts", "stdio", "127.0.0.1:8080", "", true, false, true},
		{"unknown", "tcp", "", "", true, false, true},
		{"remote on stdio", "stdio", "", "", false, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := transportOptions{transport: tt.transport, listen: tt.listen, allowRemote: tt.remote}
			got, err := opts.mode(tt.explicit, tt.listen != "")
			if (err != nil) != tt.bad || got != tt.want {
				t.Fatalf("mode = %q, %v; want %q, error=%v", got, err, tt.want, tt.bad)
			}
		})
	}
}

func TestMCPListenScope(t *testing.T) {
	for _, tt := range []struct {
		address, want      string
		allow, remote, bad bool
	}{
		{"", "127.0.0.1:8080", false, false, false},
		{"localhost:9000", "127.0.0.1:9000", false, false, false},
		{"[::1]:0", "[::1]:0", false, false, false},
		{"0.0.0.0:8080", "", false, false, true},
		{":8080", "", false, false, true},
		{"192.0.2.1:8080", "", false, false, true},
		{"0.0.0.0:8080", "0.0.0.0:8080", true, true, false},
		{"example.invalid:8080", "", true, false, true},
		{"127.0.0.1", "", false, false, true},
		{"127.0.0.1:bad", "", false, false, true},
		{"127.0.0.1:65536", "", false, false, true},
	} {
		t.Run(tt.address, func(t *testing.T) {
			address, remote, err := resolveHTTPListen(tt.address, tt.allow)
			if (err != nil) != tt.bad || address != tt.want || remote != tt.remote {
				t.Fatalf("listen = %q, remote=%v, %v", address, remote, err)
			}
		})
	}
}

func TestMCPHTTPTokenSources(t *testing.T) {
	t.Setenv("LUDUS_MCP_TOKEN", "environment-token")
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ file, want string }{{"", "environment-token"}, {path, "file-token"}} {
		got, err := loadHTTPToken(tt.file)
		if err != nil || got != tt.want {
			t.Fatalf("token source failed: %v", err)
		}
	}
}

func TestMCPHTTPRejectsInvalidTokenSources(t *testing.T) {
	for _, token := range []string{"", "two words", "line\nbreak"} {
		t.Run(token, func(t *testing.T) {
			t.Setenv("LUDUS_MCP_TOKEN", token)
			if _, err := loadHTTPToken(""); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	if _, err := loadHTTPToken(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing token file accepted")
	}
}
