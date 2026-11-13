package binary

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDestroyPlanResolvesOutputDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, tt := range []struct{ configured, want, source string }{
		{"", filepath.Join(root, "dist", "server"), "default"},
		{"package", filepath.Join(root, "package"), "config"},
		{root, root, "config"},
	} {
		plan, err := PlanDestroy(tt.configured)
		if err != nil {
			t.Fatal(err)
		}
		if item := plan.Items[0]; item.Identifier != tt.want || item.Source != tt.source {
			t.Fatalf("directory = %+v", item)
		}
	}
}

func TestDestroyPlanReportsUnavailableWorkingDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux getwd failure after removing the current directory")
	}
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanDestroy("package")
	if err == nil || !strings.Contains(err.Error(), "resolving output path") {
		t.Fatalf("missing path resolution error: %v", err)
	}
	if len(plan.Items) != 0 {
		t.Fatalf("unresolved path produced deletion candidates: %+v", plan)
	}
}
