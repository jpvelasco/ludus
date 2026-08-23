package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestGameServerKey_LyraDefaultProjectPath(t *testing.T) {
	engineDir := t.TempDir()
	lyraPath := filepath.Join(engineDir, "Samples", "Games", "Lyra", "Lyra.uproject")
	if err := os.MkdirAll(filepath.Dir(lyraPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lyraPath, []byte(`{"FileVersion": 3}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineDir, Version: "5.7.3"},
		Game: config.GameConfig{
			ProjectName:  "Lyra",
			ServerTarget: "LyraServer",
			GameTarget:   "Lyra",
			ServerMap:    "/Game/Maps/TestMap",
			Arch:         "amd64",
		},
	}

	key := GameServerKey(cfg, "engine-hash")
	if key == "" {
		t.Fatal("GameServerKey with Lyra default project path should be non-empty")
	}

	// Changing the engine source path must change the key (the Lyra default
	// project path is derived from it, and now points at a missing file).
	cfg.Engine.SourcePath = filepath.Join(t.TempDir(), "different")
	if changed := GameServerKey(cfg, "engine-hash"); changed == key {
		t.Error("GameServerKey should differ when the derived Lyra path changes")
	}
}

func TestGameClientKey_LyraDefaultProjectPath(t *testing.T) {
	engineDir := t.TempDir()
	lyraPath := filepath.Join(engineDir, "Samples", "Games", "Lyra", "Lyra.uproject")
	if err := os.MkdirAll(filepath.Dir(lyraPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lyraPath, []byte(`{"FileVersion": 3}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineDir, Version: "5.7.3"},
		Game: config.GameConfig{
			ProjectName:  "Lyra",
			ClientTarget: "LyraClient",
			ServerMap:    "/Game/Maps/TestMap",
			Arch:         "amd64",
		},
	}

	key := GameClientKey(cfg, "engine-hash", "Windows")
	if key == "" {
		t.Fatal("GameClientKey with Lyra default empty path should be non-empty")
	}
	cfg.Engine.SourcePath = filepath.Join(t.TempDir(), "different")
	if changed := GameClientKey(cfg, "engine-hash", "Windows"); changed == key {
		t.Error("GameClientKey should change when the derived Lyra path changes")
	}
}

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

// gameKeyTestProject returns a config pointing at a fake project tree plus
// the project directory, for input-change-detection tests.
func gameKeyTestProject(t *testing.T) (*config.Config, string) {
	t.Helper()
	projectPath := testsupport.FakeProject(t, "TestGame")
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir(), Version: "5.7.3"},
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ProjectPath:  projectPath,
			ServerTarget: "TestGameServer",
			GameTarget:   "TestGame",
			ServerMap:    "/Game/Maps/TestMap",
			Arch:         "amd64",
		},
	}
	return cfg, filepath.Dir(projectPath)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGameServerKey_SourceAddInvalidates pins that a new file under the
// project's Source tree changes the server cache key.
func TestGameServerKey_SourceAddInvalidates(t *testing.T) {
	cfg, root := gameKeyTestProject(t)
	before := GameServerKey(cfg, "engine-hash")

	writeFile(t, filepath.Join(root, "Source", "TestGame", "TestGame.cpp"), "int main(){}\n")
	if got := GameServerKey(cfg, "engine-hash"); got == before {
		t.Error("adding a Source file must change the server cache key")
	}
}

// TestGameServerKey_SameSizeEditInvalidates pins that the manifest tracks
// mtimes: rewriting a source file with same-size content still invalidates.
func TestGameServerKey_SameSizeEditInvalidates(t *testing.T) {
	cfg, root := gameKeyTestProject(t)
	module := filepath.Join(root, "Source", "TestGame", "TestGame.cpp")
	writeFile(t, module, "int main(){}\n")

	before := GameServerKey(cfg, "engine-hash")
	time.Sleep(20 * time.Millisecond)
	writeFile(t, module, "// x\n")
	if got := GameServerKey(cfg, "engine-hash"); got == before {
		t.Error("a same-size source edit must change the server cache key")
	}
}

// TestGameKeys_ContentAddInvalidates pins that new Content assets change both
// the server and client keys.
func TestGameKeys_ContentAddInvalidates(t *testing.T) {
	cfg, root := gameKeyTestProject(t)

	serverBefore := GameServerKey(cfg, "engine-hash")
	clientBefore := GameClientKey(cfg, "engine-hash", "Linux")

	writeFile(t, filepath.Join(root, "Content", "Props", "crate.uasset"), "crate")

	if got := GameServerKey(cfg, "engine-hash"); got == serverBefore {
		t.Error("adding a Content asset must change the server cache key")
	}
	if got := GameClientKey(cfg, "engine-hash", "Linux"); got == clientBefore {
		t.Error("adding a Content asset must change the client cache key")
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
