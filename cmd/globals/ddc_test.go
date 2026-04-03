package globals

import (
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

func TestResolveDDCMode(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		cfgMode string
		want    string
	}{
		{"default", "", "", "local"},
		{"flag local", "local", "", "local"},
		{"flag none", "none", "", "none"},
		{"config local", "", "local", "local"},
		{"config none", "", "none", "none"},
		{"flag overrides config", "none", "local", "none"},
		{"invalid flag falls back", "garbage", "", "local"},
		{"invalid config falls back", "", "garbage", "local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origMode := DDCMode
			origCfg := Cfg
			t.Cleanup(func() {
				DDCMode = origMode
				Cfg = origCfg
			})

			DDCMode = tt.flag
			Cfg = &config.Config{}
			Cfg.DDC.Mode = tt.cfgMode

			got := ResolveDDCMode()
			if got != tt.want {
				t.Errorf("ResolveDDCMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDDCPath(t *testing.T) {
	t.Run("config path", func(t *testing.T) {
		origCfg := Cfg
		t.Cleanup(func() { Cfg = origCfg })

		Cfg = &config.Config{}
		Cfg.DDC.LocalPath = "/custom/ddc"

		got := ResolveDDCPath()
		if got != "/custom/ddc" {
			t.Errorf("ResolveDDCPath() = %q, want %q", got, "/custom/ddc")
		}
	})

	t.Run("default path", func(t *testing.T) {
		origCfg := Cfg
		t.Cleanup(func() { Cfg = origCfg })

		Cfg = &config.Config{}

		got := ResolveDDCPath()
		if got == "" {
			t.Error("ResolveDDCPath() returned empty string for default")
		}
	})
}
