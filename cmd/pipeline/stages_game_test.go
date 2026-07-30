package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestStageGameBuildSkipsOnCacheHit(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:/ue5",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
		},
	}

	globals.SetGlobals(t, cfg)

	bc := newTestCache()
	serverKey := "test_server_hash"
	bc.Set(cache.StageGameServer, serverKey, "2024-01-01T00:00:00Z")

	p := &pipelineCtx{
		cfg:        cfg,
		r:          globals.NewRunner(),
		serverHash: serverKey,
		buildCache: bc,
		target:     &stubTarget{},
	}

	err := p.stageGameBuild(context.Background())
	if err != nil {
		t.Fatalf("stageGameBuild() error = %v, want nil", err)
	}
}

func TestDispatchGameBuildNative(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t)
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "native",
		engineVersion:    "5.7",
		arch:             "amd64",
		ddcMode:          "none",
		ddcPath:          "",
		target:           &stubTarget{},
	}

	err := p.dispatchGameBuild(context.Background(), "TestGame")
	if err != nil {
		t.Fatalf("dispatchGameBuild() error = %v, want nil", err)
	}
}

func TestStageClientBuildSkipsOnCacheHit(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:/ue5",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	bc := newTestCache()
	clientKey := "client_hash"
	bc.Set(cache.StageGameClient, clientKey, "2024-01-01T00:00:00Z")

	p := &pipelineCtx{
		cfg:        cfg,
		r:          globals.NewRunner(),
		clientHash: clientKey,
		buildCache: bc,
		target:     &stubTarget{},
	}

	err := p.stageClientBuild(context.Background())
	if err != nil {
		t.Fatalf("stageClientBuild() error = %v, want nil", err)
	}
}

func TestStageGameBuildNative(t *testing.T) {
	engineRoot, projectPath, cfg := setupTestContext(t, "TestGame")

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	bc := newTestCache()

	p := newTestPipelineCtx(t, cfg, &testContextOpts{
		containerBackend: "native",
		ddcMode:          "none",
		ddcPath:          "",
		withBuildCache:   true,
	})
	p.r = r
	p.serverHash = "test_hash"
	p.buildCache = bc

	err := p.stageGameBuild(context.Background())
	if err != nil {
		t.Fatalf("stageGameBuild() error = %v, want nil", err)
	}

	_ = engineRoot
	_ = projectPath
}

func TestStageClientBuildNative(t *testing.T) {
	engineRoot, projectPath, cfg := setupTestContext(t, "TestGame")

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	bc := newTestCache()

	p := newTestPipelineCtx(t, cfg, &testContextOpts{
		containerBackend: "native",
		ddcMode:          "none",
		ddcPath:          "",
		withBuildCache:   true,
	})
	p.r = r
	p.clientHash = "test_hash"
	p.buildCache = bc

	err := p.stageClientBuild(context.Background())
	if err != nil {
		t.Fatalf("stageClientBuild() error = %v, want nil", err)
	}

	_ = engineRoot
	_ = projectPath
}
