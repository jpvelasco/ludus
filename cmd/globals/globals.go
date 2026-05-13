package globals

import "github.com/devrecon/ludus/internal/config"

// Cfg holds the loaded configuration, set by root command's PersistentPreRunE.
var Cfg *config.Config

// Verbose indicates whether verbose output is enabled.
var Verbose bool

// JSONOutput indicates whether JSON output is enabled.
var JSONOutput bool

// DryRun indicates whether dry-run mode is enabled.
var DryRun bool

// Profile is the state profile name for multi-version workflows.
// Default is "" (uses .ludus/state.json). Non-empty uses .ludus/profiles/<name>.json.
var Profile string

// DDCMode is the DDC backend mode: "local" (default) or "none".
// Set via --ddc flag, overrides config file.
var DDCMode string

// ResolveBackend returns the effective build backend.
// CLI flag takes precedence over config.
func ResolveBackend(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if Cfg != nil {
		return Cfg.Engine.Backend
	}
	return ""
}

// ResolveContainerBackend returns the effective container runtime backend ("docker" or "podman").
// Unlike ResolveBackend, it ignores non-container backends like "wsl2" and "native" — those
// are engine build backends that don't apply to container image builds.
func ResolveContainerBackend(flagValue string) string {
	be := flagValue
	if be == "" && Cfg != nil {
		be = Cfg.Engine.Backend
	}
	switch be {
	case "docker", "podman":
		return be
	}
	return ""
}
