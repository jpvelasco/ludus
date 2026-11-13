package mcp

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMCPStdioCommandRestoresOutput(t *testing.T) {
	input, err := os.Create(filepath.Join(t.TempDir(), "input"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	output, err := os.Create(filepath.Join(t.TempDir(), "output"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	oldInput, oldOutput, oldBuilds := os.Stdin, os.Stdout, builds
	t.Cleanup(func() { os.Stdin, os.Stdout, builds = oldInput, oldOutput, oldBuilds })
	os.Stdin, os.Stdout = input, output
	cmd := newMCPCmd()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if os.Stdout != output {
		t.Fatal("stdio command left stdout redirected")
	}
}

func TestMCPCommandRejectsConflictingTransportFlags(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", ""} {
		cmd := newMCPCmd()
		cmd.SetArgs([]string{"--transport", "stdio", "--listen", address})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err == nil {
			t.Fatal("explicit stdio accepted an HTTP listener")
		}
	}
}

type listenNotice chan string

func (n listenNotice) Write(data []byte) (int, error) {
	n <- string(data)
	return len(data), nil
}

func TestMCPHTTPCommandUsesListenerAndRestoresOutput(t *testing.T) {
	t.Setenv("LUDUS_MCP_TOKEN", "test-token")
	oldOutput, oldBuilds := os.Stdout, builds
	t.Cleanup(func() { os.Stdout, builds = oldOutput, oldBuilds })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := newMCPCmd()
	cmd.SetArgs([]string{"--listen", "127.0.0.1:0"})
	notice := make(listenNotice, 1)
	cmd.SetErr(notice)
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- cmd.ExecuteContext(ctx)
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	var message string
	select {
	case message = <-notice:
	case <-time.After(10 * time.Second):
		t.Fatal("HTTP command did not announce its listener")
	}
	endpoint := strings.Fields(message)[5]
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	cancel()
	<-finished
	for name, ok := range map[string]bool{
		"authentication":   response.StatusCode == http.StatusUnauthorized,
		"stdout restored":  os.Stdout == oldOutput,
		"token not logged": !strings.Contains(message, "test-token"),
	} {
		if !ok {
			t.Errorf("%s failed", name)
		}
	}
}

func TestRunHTTPReportsStartupFailures(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	for _, tt := range []struct {
		name, token string
		opts        transportOptions
	}{
		{"bad address", "test-token", transportOptions{listen: "invalid"}},
		{"missing token", "", transportOptions{}},
		{"occupied port", "test-token", transportOptions{listen: listener.Addr().String()}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LUDUS_MCP_TOKEN", tt.token)
			if err := runHTTP(context.Background(), newMCPServer(), tt.opts, io.Discard); err == nil {
				t.Fatal("startup failure was ignored")
			}
		})
	}
}
