package prereq

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestCheckEngineReady(t *testing.T) {
	c := &Checker{EngineSourcePath: "/nonexistent"}
	results := c.CheckEngineReady()
	if len(results) != 1 {
		t.Fatalf("expected 1 check, got %d", len(results))
	}
	if results[0].Name == "" {
		t.Error("expected non-empty check name")
	}
}

func TestCheckGameReady(t *testing.T) {
	c := &Checker{
		EngineSourcePath: "/nonexistent",
		GameConfig:       &config.GameConfig{},
	}
	results := c.CheckGameReady()
	if len(results) != 2 {
		t.Fatalf("expected 2 checks (engine source + game content), got %d", len(results))
	}
}

func TestCheckGameReady_PrebuiltImageSkipsEngineSource(t *testing.T) {
	// With a prebuilt engine image, the build runs inside the image and does not
	// read the host engine source tree, so the Engine Source check is skipped —
	// only game content is validated.
	c := &Checker{
		EngineSourcePath: "/nonexistent",
		GameConfig:       &config.GameConfig{ProjectPath: "/some/MyGame.uproject"},
		PrebuiltImage:    true,
	}
	results := c.CheckGameReady()
	if len(results) != 1 {
		t.Fatalf("expected 1 check (game content only) with prebuilt image, got %d", len(results))
	}
	for _, r := range results {
		if r.Name == "Engine Source" {
			t.Errorf("Engine Source check should be skipped when a prebuilt image is configured")
		}
	}
}

func TestCheckDockerReady(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{}}
	results := c.CheckDockerReady()
	if len(results) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(results))
	}
}

func TestCheckPushReady(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{}}
	results := c.CheckPushReady()
	if len(results) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(results))
	}
}

func TestCheckAWSReady(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{}}
	results := c.CheckAWSReady()
	if len(results) != 1 {
		t.Fatalf("expected 1 check, got %d", len(results))
	}
}
