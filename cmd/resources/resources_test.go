package resources

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/inventory"
	"github.com/spf13/cobra"
)

func TestRunResourcesInvalidAWSConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[default]\nregion = us-east-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", path)
	t.Setenv("AWS_PROFILE", "ludus-test-missing-profile")
	t.Setenv("AWS_DEFAULT_PROFILE", "ludus-test-missing-profile")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	globals.SetGlobals(t, &config.Config{AWS: config.AWSConfig{Region: "us-east-1"}})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runResources(cmd, nil); err == nil || !strings.Contains(err.Error(), "loading AWS config") {
		t.Fatalf("expected configuration error before scanning, got %v", err)
	}
}

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

func TestResourceDetail(t *testing.T) {
	tests := []struct {
		name     string
		resource inventory.Resource
		want     string
	}{
		{"detail takes precedence", inventory.Resource{Detail: "detail", Status: "status"}, "detail"},
		{"status fallback", inventory.Resource{Status: "ready"}, "ready"},
		{"empty", inventory.Resource{}, "--"},
		{"configured name", inventory.Resource{Scope: "configured-name", Detail: "5 images"}, "configured name (ownership unverified); 5 images"},
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

func TestPrintWarnings(t *testing.T) {
	inv := &inventory.Inventory{Warnings: []string{"failed to describe ECR repository ludus-server: boom"}}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	printWarnings(inv)
	w.Close()
	os.Stderr = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Warning: failed to describe ECR repository ludus-server") {
		t.Errorf("stderr = %q, want warning line", string(out))
	}
}
