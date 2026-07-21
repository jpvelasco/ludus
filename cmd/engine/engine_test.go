package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/toolchain"
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

func TestMakeContainerEngineBuilderPreservesFullVersion(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Engine.Version = "5.7.4"
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("docker")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	tag := builder.FullImageTag()
	if tag != "ludus-engine:5.7.4" {
		t.Errorf("FullImageTag() = %q, want ludus-engine:5.7.4", tag)
	}
}

func TestMakeContainerEngineBuilderCustomImageName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Engine.Version = "5.7.4"
	cfg.Engine.DockerImageName = "my-registry/ludus-engine"
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("podman")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	tag := builder.FullImageTag()
	if tag != "my-registry/ludus-engine:5.7.4" {
		t.Errorf("FullImageTag() = %q, want my-registry/ludus-engine:5.7.4", tag)
	}
}

// TestMakeContainerEngineBuilderOverrideFromBuildVersion verifies that when
// the engine source path contains a Build.version file with a different patch
// than cfg.Engine.Version, the image tag is derived from the actual source.
func TestMakeContainerEngineBuilderOverrideFromBuildVersion(t *testing.T) {
	srcDir := t.TempDir()

	// Write a Build.version file matching 5.8.2
	bv := toolchain.BuildVersion{MajorVersion: 5, MinorVersion: 8, PatchVersion: 2}
	err := os.MkdirAll(filepath.Join(srcDir, "Engine", "Build"), 0755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "Engine", "Build", "Build.version"),
		mustMarshalJSON(t, bv), 0644); err != nil {
		t.Fatalf("write Build.version: %v", err)
	}

	cfg := &config.Config{}
	cfg.Engine.SourcePath = srcDir
	cfg.Engine.Version = "5.7.4" // config says 5.7.4, but source is 5.8.2
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("docker")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	// The tag should be derived from Build.version (5.8.2), not from config (5.7.4)
	tag := builder.FullImageTag()
	if tag != "ludus-engine:5.8.2" {
		t.Errorf("FullImageTag() = %q, want ludus-engine:5.8.2", tag)
	}
}

// TestMakeContainerEngineBuilderFallbackToConfig verifies that when the engine
// source path has no Build.version file, the image tag falls back to
// cfg.Engine.Version.
func TestMakeContainerEngineBuilderFallbackToConfig(t *testing.T) {
	srcDir := t.TempDir() // empty dir, no Build.version

	cfg := &config.Config{}
	cfg.Engine.SourcePath = srcDir
	cfg.Engine.Version = "5.7.4"
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("docker")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	tag := builder.FullImageTag()
	if tag != "ludus-engine:5.7.4" {
		t.Errorf("FullImageTag() = %q, want ludus-engine:5.7.4", tag)
	}
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
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
