package mcp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPAuthentication(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers []string
		want    int
	}{
		{"missing", nil, http.StatusUnauthorized},
		{"wrong", []string{"Bearer wrong"}, http.StatusUnauthorized},
		{"wrong scheme", []string{"Basic test-token"}, http.StatusUnauthorized},
		{"empty", []string{"Bearer "}, http.StatusUnauthorized},
		{"duplicate", []string{"Bearer test-token", "Bearer test-token"}, http.StatusUnauthorized},
		{"valid", []string{"Bearer test-token"}, http.StatusNoContent},
		{"case insensitive scheme", []string{"bearer test-token"}, http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := requireBearerToken(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}), "test-token")
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			req.Header["Authorization"] = tt.headers
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status=%d; want %d", rec.Code, tt.want)
			}
			if rec.Code == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("missing bearer challenge")
			}
		})
	}
}

func TestHTTPOriginAndHostProtection(t *testing.T) {
	handler := newHTTPHandler(newMCPServer(), "test-token")
	for _, tt := range []struct{ name, host, origin string }{
		{"cross origin", "localhost:8080", "https://untrusted.invalid"},
		{"nonlocal host", "untrusted.invalid", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://"+tt.host+"/mcp", strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Origin", tt.origin)
			req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("unsafe request status=%d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

type bearerTransport struct{ base http.RoundTripper }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer test-token")
	return b.base.RoundTrip(r)
}

func TestStreamableHTTPClientCanUseLudusTools(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, config.Defaults())
	server := httptest.NewServer(newHTTPHandler(newMCPServer(), "test-token"))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test", Version: "test"}, nil)
	httpClient := server.Client()
	httpClient.Transport = bearerTransport{httpClient.Transport}
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: httpClient}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tool list = %+v, %v", tools, err)
	}
	assertRegisteredTools(t, tools.Tools)
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "ludus_deploy_session", Arguments: map[string]any{"dry_run": true},
	})
	if err != nil || result.IsError {
		t.Fatalf("dry-run call = %+v, %v", result, err)
	}
}

func TestHTTPShutdownCancelsRequests(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, stopped := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(stopped)
	})
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, listener, handler) }()
	client := &http.Client{Timeout: 10 * time.Second}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("request never reached server")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	<-stopped
	<-requestDone
}

func TestHTTPServeReportsListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
	if err := serveHTTP(context.Background(), listener, http.NotFoundHandler()); err == nil {
		t.Fatal("listener failure was reported as a successful shutdown")
	}
}
