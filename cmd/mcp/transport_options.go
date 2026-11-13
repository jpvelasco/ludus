package mcp

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"unicode"
)

type transportOptions struct {
	transport   string
	listen      string
	tokenFile   string
	allowRemote bool
}

func (o transportOptions) mode(explicit, listenSet bool) (string, error) {
	mode := o.transport
	if listenSet && !explicit {
		mode = "http"
	}
	switch mode {
	case "http":
		return mode, nil
	case "stdio":
		if listenSet || o.tokenFile != "" || o.allowRemote {
			return "", fmt.Errorf("--listen, --token-file, and --allow-remote require HTTP transport")
		}
		return mode, nil
	default:
		return "", fmt.Errorf("unknown MCP transport %q; use stdio or http", mode)
	}
}

// resolveHTTPListen uses literal addresses so DNS cannot change the bind scope.
func resolveHTTPListen(address string, allowRemote bool) (string, bool, error) {
	if address == "" {
		address = "127.0.0.1:8080"
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", false, fmt.Errorf("MCP listen address must be host:port: %w", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 0 || number > 65535 {
		return "", false, fmt.Errorf("MCP listen port must be a number from 0 to 65535")
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if host != "" && ip == nil {
		return "", false, fmt.Errorf("MCP listen host must be a literal IP address or localhost")
	}
	remote := !ip.IsLoopback()
	if remote && !allowRemote {
		return "", false, fmt.Errorf("non-loopback MCP listener requires --allow-remote; prefer loopback with port forwarding")
	}
	return net.JoinHostPort(host, port), remote, nil
}

func loadHTTPToken(file string) (string, error) {
	token := os.Getenv("LUDUS_MCP_TOKEN")
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading MCP token file: %w", err)
		}
		token = string(data)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("HTTP MCP requires LUDUS_MCP_TOKEN or --token-file")
	}
	if strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", fmt.Errorf("MCP bearer token must not contain whitespace or control characters")
	}
	return token, nil
}
