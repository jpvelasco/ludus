package globals

import (
	"github.com/jpvelasco/ludus/internal/config"
)

// ApplyEngineOverrides writes the effective engine options into cfg: a
// non-empty CLI/MCP override wins, otherwise the configured value stands.
// Prerequisite validation, cache-key hashing, and builder construction must
// all read the same resolved config, so a --backend or --path override cannot
// desynchronize the cache key from the build that actually runs (#728, #729).
func ApplyEngineOverrides(cfg *config.Config, backend, sourcePath string) {
	if backend != "" {
		cfg.Engine.Backend = backend
	}
	if sourcePath != "" {
		cfg.Engine.SourcePath = sourcePath
	}
}
