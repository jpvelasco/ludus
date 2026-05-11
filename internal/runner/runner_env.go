package runner

import (
	"os"
	"strings"
)

// environ returns the merged environment for child processes. If Env is empty,
// it returns nil so exec.Cmd inherits the parent environment unchanged.
// When Env is set, parent variables are included and any matching keys are
// overridden by the Env values.
func (r *Runner) environ() []string {
	if len(r.Env) == 0 {
		return nil
	}

	// Build a set of override keys for quick lookup.
	overrides := make(map[string]string, len(r.Env))
	for _, kv := range r.Env {
		if k, _, ok := strings.Cut(kv, "="); ok {
			overrides[k] = kv
		}
	}

	// Start with parent env, replacing any keys present in overrides.
	parent := os.Environ()
	merged := make([]string, 0, len(parent)+len(r.Env))
	seen := make(map[string]bool, len(overrides))
	for _, kv := range parent {
		k, _, _ := strings.Cut(kv, "=")
		if override, ok := overrides[k]; ok {
			merged = append(merged, override)
			seen[k] = true
		} else {
			merged = append(merged, kv)
		}
	}

	// Append any override keys that weren't in the parent env.
	for k, kv := range overrides {
		if !seen[k] {
			merged = append(merged, kv)
		}
	}

	return merged
}
