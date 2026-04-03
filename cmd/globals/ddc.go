package globals

import (
	"fmt"

	"github.com/devrecon/ludus/internal/ddc"
)

// ResolveDDCMode returns the effective DDC mode.
// CLI flag (DDCMode) takes precedence over config (Cfg.DDC.Mode).
// Invalid values are rejected with a warning and fall back to "local".
func ResolveDDCMode() string {
	var mode string
	if DDCMode != "" {
		mode = DDCMode
	} else if Cfg != nil && Cfg.DDC.Mode != "" {
		mode = Cfg.DDC.Mode
	}
	validated, err := ddc.ValidateDDCMode(mode)
	if err != nil {
		fmt.Printf("  Warning: %v, using default 'local'\n", err)
		return "local"
	}
	return validated
}

// ResolveDDCPath returns the effective DDC host path.
// Config local_path takes precedence over the default path.
func ResolveDDCPath() string {
	if Cfg != nil && Cfg.DDC.LocalPath != "" {
		return Cfg.DDC.LocalPath
	}
	p, err := ddc.DefaultPath()
	if err != nil {
		fmt.Printf("  Warning: could not resolve DDC path: %v\n", err)
		return ""
	}
	return p
}
