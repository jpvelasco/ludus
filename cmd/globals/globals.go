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
