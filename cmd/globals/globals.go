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
