package prereq

import (
	"testing"

	"github.com/devrecon/ludus/internal/config"
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
		t.Fatalf("expected 2 checks, got %d", len(results))
	}
}

func TestCheckDockerReady(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{}}
	results := c.CheckDockerReady()
	if len(results) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(results))
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
