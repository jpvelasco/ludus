package globals

// CanSaveBuildState reports whether a successful build may persist its
// results to .ludus/state.json. A dry-run preview reports success without
// building anything, so recording it would replace valid saved outputs with
// paths and images that were never built (#697, #733). MCP tool handlers
// pass the effective per-request value: the schema dry_run input OR this
// global flag; CLI commands pass the flag directly.
func CanSaveBuildState(effectiveDryRun bool) bool {
	return !effectiveDryRun
}
