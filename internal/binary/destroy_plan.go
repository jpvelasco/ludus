package binary

import (
	"fmt"
	"path/filepath"

	"github.com/jpvelasco/ludus/internal/deploy"
)

// ResolveOutputDir supplies the same default for export and offline teardown.
func ResolveOutputDir(configured string) string {
	if configured == "" {
		return "./dist/server"
	}
	return configured
}

// PlanDestroy resolves the same absolute path as Exporter.Destroy, without
// inspecting builds or changing the directory.
func PlanDestroy(configured string) (deploy.DestroyTargetPlan, error) {
	path, err := filepath.Abs(ResolveOutputDir(configured))
	if err != nil {
		return deploy.DestroyTargetPlan{}, fmt.Errorf("resolving output path: %w", err)
	}
	source := deploy.SourceConfig
	if configured == "" {
		source = deploy.SourceDefault
	}
	return deploy.DestroyTargetPlan{Target: "binary", Items: []deploy.DestroyItem{{
		Kind: "directory", Identifier: path, Source: source,
		Action: deploy.ActionDelete, Decision: deploy.DecisionPlanned,
		Reason: "Deletion is recursive and includes all contents; an absent directory is a no-op.",
	}}}, nil
}
