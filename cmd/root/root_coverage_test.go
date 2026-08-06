package root

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	ddcpkg "github.com/jpvelasco/ludus/internal/ddc"
)

func TestPersistentPreRunConfigLoadError(t *testing.T) {
	configPath := writeRootConfig(t, "not: :: [valid] yaml")
	setRootGlobals(t)
	cfgFile = configPath

	err := rootCmd.PersistentPreRunE(namedRootCommand("status"), nil)
	if err == nil {
		t.Fatal("PersistentPreRunE() expected config load error, got nil")
	}
}

func TestPersistentPreRunWarnsLegacyDDCOnNonMCP(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	configPath := writeRootConfig(t, "ddc:\n  mode: local\ndeploy:\n  target: binary\n")
	setRootGlobals(t)
	cfgFile = configPath

	// Non-mcp command so WarnIfLegacyDDC actually runs and reads the config
	// mode "local" from the loaded Cfg.
	stderr, err := captureRootStderr(t, func() error {
		return rootCmd.PersistentPreRunE(namedRootCommand("status"), nil)
	})
	if err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}
	if !strings.Contains(stderr, ddcpkg.LocalModeDeprecationWarning) {
		t.Errorf("stderr = %q, want legacy DDC warning", stderr)
	}
}

func TestWarnIfLegacyDDC(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		cfgMode string
		want    bool
	}{
		{name: "flag local", flag: ddcpkg.ModeLocal, want: true},
		{name: "config local", cfgMode: ddcpkg.ModeLocal, want: true},
		{name: "zen no warning", want: false},
		{name: "none no warning", cfgMode: ddcpkg.ModeNone, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, &config.Config{DDC: config.DDCConfig{Mode: tt.cfgMode}})
			globals.DDCMode = tt.flag

			stderr, _ := captureRootStderr(t, func() error {
				globals.WarnIfLegacyDDC()
				return nil
			})
			got := strings.Contains(stderr, ddcpkg.LocalModeDeprecationWarning)
			if got != tt.want {
				t.Errorf("WarnIfLegacyDDC() warning present = %v, want %v (stderr=%q)", got, tt.want, stderr)
			}
		})
	}
}
