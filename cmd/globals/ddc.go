package globals

import "github.com/devrecon/ludus/internal/ddc"

// ResolveDDCMode returns the effective DDC mode.
// CLI flag (DDCMode) takes precedence over config (Cfg.DDC.Mode).
func ResolveDDCMode() string {
	if DDCMode != "" {
		return DDCMode
	}
	if Cfg != nil && Cfg.DDC.Mode != "" {
		return Cfg.DDC.Mode
	}
	return "local"
}

// ResolveDDCPath returns the effective DDC host path.
// Config local_path takes precedence over the default path.
func ResolveDDCPath() string {
	if Cfg != nil && Cfg.DDC.LocalPath != "" {
		return Cfg.DDC.LocalPath
	}
	p, err := ddc.DefaultPath()
	if err != nil {
		return ""
	}
	return p
}
