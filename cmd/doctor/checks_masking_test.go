package doctor

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestCheckAccountIDMasking(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		showFlag    bool
		wantStatus  string
		wantMessage string
	}{
		{
			name:        "enabled by default",
			cfg:         &config.Config{Privacy: config.PrivacyConfig{MaskAccountID: true}},
			wantStatus:  "ok",
			wantMessage: "enabled",
		},
		{
			name:        "disabled via config",
			cfg:         &config.Config{Privacy: config.PrivacyConfig{MaskAccountID: false}},
			wantStatus:  "ok",
			wantMessage: "privacy.maskAccountId=false",
		},
		{
			name:        "disabled via flag",
			cfg:         &config.Config{Privacy: config.PrivacyConfig{MaskAccountID: true}},
			showFlag:    true,
			wantStatus:  "ok",
			wantMessage: "--show-account-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := globals.ShowAccountID
			t.Cleanup(func() { globals.ShowAccountID = orig })
			globals.ShowAccountID = tt.showFlag

			got := checkAccountIDMasking(tt.cfg)
			if got.status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.status, tt.wantStatus)
			}
			if !strings.Contains(got.message, tt.wantMessage) {
				t.Errorf("message = %q, want substring %q", got.message, tt.wantMessage)
			}
		})
	}
}
