package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

var expectedToolNames = map[string]bool{
	"ludus_init": true, "ludus_status": true,
	"ludus_engine_setup": true, "ludus_engine_build": true, "ludus_engine_push": true,
	"ludus_game_build": true, "ludus_game_client": true,
	"ludus_container_build": true, "ludus_container_push": true,
	"ludus_deploy_fleet": true, "ludus_deploy_stack": true, "ludus_deploy_anywhere": true,
	"ludus_deploy_ec2": true, "ludus_deploy_session": true, "ludus_deploy_destroy": true,
	"ludus_connect_info": true, "ludus_engine_build_start": true,
	"ludus_game_build_start": true, "ludus_game_client_start": true, "ludus_build_status": true,
	"ludus_buildgraph": true, "ludus_resources": true,
	"ludus_ddc_status": true, "ludus_ddc_clean": true, "ludus_ddc_configure": true, "ludus_ddc_warm": true,
}

func TestRegisterToolsCompletenessAndSchemas(t *testing.T) {
	ctx := context.Background()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "test"}, nil)
	registerTools(server)
	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	})

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	assertRegisteredTools(t, result.Tools)
}

func assertRegisteredTools(t *testing.T, tools []*sdkmcp.Tool) {
	t.Helper()
	if len(tools) != len(expectedToolNames) {
		t.Errorf("tool count = %d, want %d", len(tools), len(expectedToolNames))
	}
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if seen[tool.Name] {
			t.Errorf("duplicate tool %q", tool.Name)
		}
		seen[tool.Name] = true
		assertToolMetadata(t, tool)
	}
	for name := range expectedToolNames {
		if !seen[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func assertToolMetadata(t *testing.T, tool *sdkmcp.Tool) {
	t.Helper()
	if !expectedToolNames[tool.Name] {
		t.Errorf("unexpected tool %q", tool.Name)
	}
	if tool.Description == "" {
		t.Errorf("tool %q has no description", tool.Name)
	}
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Errorf("tool %q schema marshal: %v", tool.Name, err)
		return
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Errorf("tool %q schema decode: %v", tool.Name, err)
		return
	}
	if schema["type"] != "object" {
		t.Errorf("tool %q schema type = %v, want object", tool.Name, schema["type"])
	}
	if properties, exists := schema["properties"]; exists {
		if _, ok := properties.(map[string]any); !ok {
			t.Errorf("tool %q schema properties = %T, want object", tool.Name, properties)
		}
	}
}
