package doctor

import (
	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

// checkAccountIDMasking reports whether AWS account IDs are masked in
// human-readable output. The --show-account-id flag overrides the config.
func checkAccountIDMasking(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Account ID Masking", status: "ok"}

	switch {
	case globals.ShowAccountID:
		d.message = "disabled for this run (--show-account-id)"
	case cfg != nil && !cfg.Privacy.MaskAccountID:
		d.message = "disabled (privacy.maskAccountId=false)"
	default:
		d.message = "enabled (account IDs masked in output)"
	}

	return d
}
