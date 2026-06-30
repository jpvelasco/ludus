package resources

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/inventory"
)

func TestResolveRegion(t *testing.T) {
	previous := regionFlag
	t.Cleanup(func() { regionFlag = previous })

	tests := []struct {
		name   string
		flag   string
		config string
		want   string
	}{
		{"config fallback", "", "us-west-2", "us-west-2"},
		{"flag override", "eu-west-1", "us-west-2", "eu-west-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regionFlag = tt.flag
			if got := resolveRegion(tt.config); got != tt.want {
				t.Fatalf("resolveRegion(%q) = %q, want %q", tt.config, got, tt.want)
			}
		})
	}
}

func TestResolveECRRepos(t *testing.T) {
	tests := []struct {
		name   string
		server string
		engine string
		want   string
	}{
		{"defaults", "", "", "ludus-server,ludus-engine"},
		{"custom", "server", "engine", "server,engine"},
		{"deduplicates", "shared", "shared", "shared"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(resolveECRRepos(tt.server, tt.engine), ",")
			if got != tt.want {
				t.Fatalf("resolveECRRepos(%q, %q) = %q, want %q", tt.server, tt.engine, got, tt.want)
			}
		})
	}
}

func TestResourceDetail(t *testing.T) {
	tests := []struct {
		name     string
		resource inventory.Resource
		want     string
	}{
		{"detail takes precedence", inventory.Resource{Detail: "detail", Status: "status"}, "detail"},
		{"status fallback", inventory.Resource{Status: "ready"}, "ready"},
		{"empty", inventory.Resource{}, "--"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resourceDetail(tt.resource); got != tt.want {
				t.Fatalf("resourceDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintInventory(t *testing.T) {
	tests := []struct {
		name      string
		inventory *inventory.Inventory
		want      []string
	}{
		{
			name:      "empty",
			inventory: &inventory.Inventory{},
			want:      []string{"No ludus-managed resources found in us-west-2."},
		},
		{
			name: "resources",
			inventory: &inventory.Inventory{Resources: []inventory.Resource{
				{Type: "ECR Repository", Name: "ludus-server", Detail: "3 images"},
				{Type: "GameLift Fleet", Name: "fleet", Status: "ACTIVE"},
			}},
			want: []string{"Ludus Resources (us-west-2)", "TYPE", "ludus-server", "3 images", "fleet", "ACTIVE"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureInventoryOutput(t, func() { printInventory(tt.inventory, "us-west-2") })
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("output %q does not contain %q", output, want)
				}
			}
		})
	}
}

func captureInventoryOutput(t *testing.T, run func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	t.Cleanup(func() { os.Stdout = previous })
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
