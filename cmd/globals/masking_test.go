package globals

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestMaskAccountIDEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		showFlag bool
		want     bool
	}{
		{
			name: "masking on by default",
			cfg:  &config.Config{Privacy: config.PrivacyConfig{MaskAccountID: true}},
			want: true,
		},
		{
			name:     "flag overrides config",
			cfg:      &config.Config{Privacy: config.PrivacyConfig{MaskAccountID: true}},
			showFlag: true,
			want:     false,
		},
		{
			name: "config disabled",
			cfg:  &config.Config{Privacy: config.PrivacyConfig{MaskAccountID: false}},
			want: false,
		},
		{
			name: "nil config",
			cfg:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCfg, origFlag := Cfg, ShowAccountID
			t.Cleanup(func() { Cfg, ShowAccountID = origCfg, origFlag })

			Cfg = tt.cfg
			ShowAccountID = tt.showFlag

			if got := MaskAccountIDEnabled(); got != tt.want {
				t.Errorf("MaskAccountIDEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
