package binary

import (
	"path/filepath"
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
