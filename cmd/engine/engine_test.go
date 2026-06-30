package engine

import (
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestMakeBuilderRequiresSourcePath(t *testing.T) {
	setEngineTestGlobals(t, &config.Config{})

	if _, err := makeBuilder(); err == nil {
		t.Fatal("makeBuilder() error = nil, want missing source path error")
	}
}

func TestMakeContainerEngineBuilderRequiresSourcePath(t *testing.T) {
	setEngineTestGlobals(t, &config.Config{})

	if _, err := makeContainerEngineBuilder("docker"); err == nil {
		t.Fatal("makeContainerEngineBuilder() error = nil, want missing source path error")
	}
}

func TestMakeBuildersWithConfiguredSource(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Engine.MaxJobs = 7
	setEngineTestGlobals(t, cfg)

	if _, err := makeBuilder(); err != nil {
		t.Fatalf("makeBuilder() error = %v", err)
	}
	if _, err := makeContainerEngineBuilder("docker"); err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}
}

func TestMaybeRunMacOSPreflightsIsNoopOnLinuxAndWindows(t *testing.T) {
	setEngineTestGlobals(t, &config.Config{})

	if err := maybeRunMacOSPreflights(t.Context()); err != nil {
		t.Fatalf("maybeRunMacOSPreflights() error = %v", err)
	}
}

func setEngineTestGlobals(t *testing.T, cfg *config.Config) {
	t.Helper()

	oldCfg := globals.Cfg
	oldUEPath, oldJobs, oldBackend := uePath, jobs, backend
	oldBaseImage := baseImage
	globals.Cfg = cfg
	uePath, jobs, backend, baseImage = "", 0, "", ""
	t.Cleanup(func() {
		globals.Cfg = oldCfg
		uePath, jobs, backend, baseImage = oldUEPath, oldJobs, oldBackend, oldBaseImage
	})
}
