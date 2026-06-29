package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/ludus/internal/config"
)

// Build-args schema versions for the game build stages. Bump the relevant token
// whenever that stage's BuildCookRun args change in a way that alters the
// packaged output, so a warm .ludus/cache.json entry does not skip the rebuild
// and keep deploying a stale package. Server and client are versioned separately
// so a server-only change does not needlessly invalidate the client cache.
//   - server-v2: added -pak -iostore (self-contained packaging, #406)
//   - client-v1: introduces the schema mechanism on the client key for future
//     protection; client args are unchanged in this PR, so existing client
//     caches take a one-time invalidation on upgrade. This is the standard
//     (one-time) cost of introducing cache versioning — omitting it now would
//     leave the client path with the #409 hole and defer the same miss to later.
const (
	serverBuildArgsSchema = "server-v2"
	clientBuildArgsSchema = "client-v1"
)

// hash computes a SHA-256 hex digest from a list of key-value strings.
func hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0}) // separator
	}
	return hex.EncodeToString(h.Sum(nil))
}

// EngineKey computes the cache key for the engine build stage.
func EngineKey(cfg *config.Config) string {
	return hash(
		gitHEAD(cfg.Engine.SourcePath),
		cfg.Engine.Version,
		fmt.Sprintf("%d", cfg.Engine.MaxJobs),
		runtime.GOOS,
		cfg.Engine.Backend,
		cfg.Engine.DockerBaseImage,
		cfg.Game.ResolvedArch(), // container builds are arch-specific
	)
}

// GameServerKey computes the cache key for the game server build stage.
func GameServerKey(cfg *config.Config, engineHash string) string {
	projectPath := cfg.Game.ProjectPath
	if projectPath == "" && cfg.Game.ProjectName == "Lyra" {
		projectPath = filepath.Join(cfg.Engine.SourcePath,
			"Samples", "Games", "Lyra", "Lyra.uproject")
	}

	return hash(
		engineHash,
		fileKey(projectPath),
		cfg.Game.ResolvedServerTarget(),
		cfg.Game.ResolvedGameTarget(),
		cfg.Game.ServerMap,
		fmt.Sprintf("%v", cfg.Game.SkipCook),
		cfg.Engine.Version,
		cfg.Game.ResolvedArch(),
		serverBuildArgsSchema,
	)
}

// GameClientKey computes the cache key for the game client build stage.
func GameClientKey(cfg *config.Config, engineHash string, platform string) string {
	projectPath := cfg.Game.ProjectPath
	if projectPath == "" && cfg.Game.ProjectName == "Lyra" {
		projectPath = filepath.Join(cfg.Engine.SourcePath,
			"Samples", "Games", "Lyra", "Lyra.uproject")
	}

	return hash(
		engineHash,
		fileKey(projectPath),
		cfg.Game.ResolvedClientTarget(),
		platform,
		fmt.Sprintf("%v", cfg.Game.SkipCook),
		cfg.Engine.Version,
		cfg.Game.ResolvedArch(),
		clientBuildArgsSchema,
	)
}

// ContainerKey computes the cache key for the container build stage.
// It hashes a manifest of filenames and sizes in the server build directory.
func ContainerKey(cfg *config.Config, serverBuildDir string) string {
	manifest := dirManifest(serverBuildDir)
	return hash(
		manifest,
		cfg.Game.ProjectName,
		cfg.Game.ResolvedServerTarget(),
		fmt.Sprintf("%d", cfg.Container.ServerPort),
		cfg.Container.Tag,
	)
}
