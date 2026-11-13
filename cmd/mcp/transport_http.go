package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func runHTTP(ctx context.Context, server *mcp.Server, opts transportOptions, out io.Writer) error {
	address, remote, err := resolveHTTPListen(opts.listen, opts.allowRemote)
	if err != nil {
		return err
	}
	token, err := loadHTTPToken(opts.tokenFile)
	if err != nil {
		return err
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("listening for MCP HTTP: %w", err)
	}
	if remote {
		fmt.Fprintln(out, "Warning: MCP is listening beyond loopback. Tools can create and destroy billable resources; use a trusted TLS proxy or encrypted tunnel.")
	}
	fmt.Fprintf(out, "MCP Streamable HTTP listening at http://%s/mcp (bearer token required)\n", listener.Addr())
	return serveHTTP(ctx, listener, newHTTPHandler(server, token))
}

func newHTTPHandler(server *mcp.Server, token string) http.Handler {
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, PropagateRequestCancellation: true,
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.NewCrossOriginProtection().Handler(stream))
	return requireBearerToken(mux, token)
}

func requireBearerToken(next http.Handler, token string) http.Handler {
	expected := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header.Values("Authorization")
		scheme, supplied, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		actual := sha256.Sum256([]byte(supplied))
		if len(headers) != 1 || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ludus-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveHTTP bounds shutdown without imposing a response timeout on long tools.
func serveHTTP(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	defer func() { _ = server.Close() }()
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(stopped)
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	})
	defer func() {
		if !stop() {
			<-stopped
		}
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving MCP HTTP: %w", err)
	}
	return nil
}
