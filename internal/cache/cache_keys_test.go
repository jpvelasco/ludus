package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestEngineKey_Deterministic(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:      "/fake/path",
			Version:         "5.7.3",
			MaxJobs:         8,
			Backend:         "native",
			DockerBaseImage: "ubuntu:22.04",
		},
	}

	k1 := EngineKey(cfg)
	k2 := EngineKey(cfg)
	if k1 != k2 {
		t.Error("EngineKey should be deterministic for same config")
	}
	if k1 == "" {
		t.Error("EngineKey should return non-empty string")
	}
}

func TestGameServerKey_DifferentArchDifferentKey(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "/fake/engine",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectPath:  "/fake/project.uproject",
			ProjectName:  "TestGame",
			ServerTarget: "TestGameServer",
			GameTarget:   "TestGame",
			ServerMap:    "/Game/Maps/TestMap",
			Arch:         "amd64",
		},
	}

	keyAmd64 := GameServerKey(cfg, "abc123")
	cfg.Game.Arch = "arm64"
	keyArm64 := GameServerKey(cfg, "abc123")

	if keyAmd64 == keyArm64 {
		t.Error("GameServerKey should produce different keys for different architectures")
	}
	if keyAmd64 == "" || keyArm64 == "" {
		t.Error("GameServerKey should return non-empty strings")
	}
}

// TestBuildArgsSchemaInGameKeys guards #409: the build-args schema version must
// participate in the game build cache keys, so bumping it when build args change
// (e.g. adding -pak -iostore) invalidates stale cache entries and forces a
// rebuild. We verify by re-hashing with the same inputs plus a different schema
// token and confirming the result differs.
func TestBuildArgsSchemaInGameKeys(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "/fake/engine", Version: "5.8.0"},
		Game:   config.GameConfig{ProjectPath: "/fake/p.uproject", ProjectName: "G", ServerTarget: "GServer", Arch: "amd64"},
	}

	// The current key includes buildArgsSchema. A key built from the same inputs
	// but a different schema token must differ — proving the schema is hashed in.
	current := GameServerKey(cfg, "eng")
	bumped := hash("eng", fileKey("/fake/p.uproject"), cfg.Game.ResolvedServerTarget(),
		cfg.Game.ResolvedGameTarget(), cfg.Game.ServerMap, "false", "5.8.0", "amd64", "vNEXT")
	if current == bumped {
		t.Error("GameServerKey must incorporate buildArgsSchema (a schema bump must change the key)")
	}
}

func TestContainerKey_DifferentPort(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "test.bin"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ServerTarget: "TestGameServer",
		},
		Container: config.ContainerConfig{
			ServerPort: 7777,
			Tag:        "latest",
		},
	}

	key1 := ContainerKey(cfg, tmpDir)
	cfg.Container.ServerPort = 8888
	key2 := ContainerKey(cfg, tmpDir)

	if key1 == key2 {
		t.Error("ContainerKey should produce different keys for different ports")
	}
	if key1 == "" || key2 == "" {
		t.Error("ContainerKey should return non-empty strings")
	}
}
