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

// TestBuildArgsSchemaInGameKeys guards #409: the build-args schema token must
// participate in the game build cache keys, so bumping it when build args change
// (e.g. adding -pak -iostore) invalidates stale cache entries and forces a
// rebuild. The assertion compares against the key WITHOUT any schema token, so
// it fails if GameServerKey/GameClientKey ever stop hashing the schema (a weaker
// "differs from a different token" check would pass even if the schema were
// dropped entirely — Codex #409).
func TestBuildArgsSchemaInGameKeys(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "/fake/engine", Version: "5.8.0"},
		Game:   config.GameConfig{ProjectPath: "/fake/p.uproject", ProjectName: "G", ServerTarget: "GServer", Arch: "amd64"},
	}

	t.Run("server key hashes its schema", func(t *testing.T) {
		current := GameServerKey(cfg, "eng")
		noSchema := hash("eng", fileKey("/fake/p.uproject"), cfg.Game.ResolvedServerTarget(),
			cfg.Game.ResolvedGameTarget(), cfg.Game.ServerMap, "false", "5.8.0", "amd64")
		if current == noSchema {
			t.Error("GameServerKey must hash serverBuildArgsSchema (dropping it must change the key)")
		}
	})

	t.Run("client key hashes its schema", func(t *testing.T) {
		current := GameClientKey(cfg, "eng", "Linux")
		noSchema := hash("eng", fileKey("/fake/p.uproject"), cfg.Game.ResolvedClientTarget(),
			"Linux", "false", "5.8.0", "amd64")
		if current == noSchema {
			t.Error("GameClientKey must hash clientBuildArgsSchema (dropping it must change the key)")
		}
	})

	t.Run("server and client schemas are independent", func(t *testing.T) {
		// A server-only schema bump must not change the client key.
		if serverBuildArgsSchema == clientBuildArgsSchema {
			t.Error("server and client schema tokens must be distinct so a server-only bump doesn't invalidate the client cache")
		}
	})
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
