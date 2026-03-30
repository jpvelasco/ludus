package binary

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devrecon/ludus/internal/deploy"
)

func TestName(t *testing.T) {
	e := NewExporter("out")
	if e.Name() != "binary" {
		t.Errorf("expected name 'binary', got %q", e.Name())
	}
}

func TestCapabilities(t *testing.T) {
	e := NewExporter("out")
	caps := e.Capabilities()
	if caps.NeedsContainerBuild {
		t.Error("binary target should not need container build")
	}
	if caps.NeedsContainerPush {
		t.Error("binary target should not need container push")
	}
	if caps.SupportsSession {
		t.Error("binary target should not support sessions")
	}
	if !caps.SupportsDeploy {
		t.Error("binary target should support deploy")
	}
	if !caps.SupportsDestroy {
		t.Error("binary target should support destroy")
	}
}

func TestDeploy_CopiesFiles(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "server.exe"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(srcDir, "Data")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "asset.pak"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "output")
	e := NewExporter(outDir)

	result, err := e.Deploy(context.Background(), deploy.DeployInput{
		ServerBuildDir: srcDir,
	})
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if result.Status != "exported" {
		t.Errorf("expected status 'exported', got %q", result.Status)
	}

	if _, err := os.Stat(filepath.Join(outDir, "server.exe")); err != nil {
		t.Error("server.exe not copied")
	}
	if _, err := os.Stat(filepath.Join(outDir, "Data", "asset.pak")); err != nil {
		t.Error("Data/asset.pak not copied")
	}
}

func TestDeploy_MissingBuildDir(t *testing.T) {
	e := NewExporter(t.TempDir())
	_, err := e.Deploy(context.Background(), deploy.DeployInput{
		ServerBuildDir: "",
	})
	if err == nil {
		t.Fatal("expected error for empty build dir")
	}
}

func TestDeploy_NonexistentBuildDir(t *testing.T) {
	e := NewExporter(t.TempDir())
	_, err := e.Deploy(context.Background(), deploy.DeployInput{
		ServerBuildDir: filepath.Join(t.TempDir(), "nonexistent"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent build dir")
	}
}

func TestStatus_Active(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	e := NewExporter(dir)
	s, err := e.Status(context.Background())
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if s.Status != "active" {
		t.Errorf("expected status 'active', got %q", s.Status)
	}
}

func TestStatus_NotDeployed(t *testing.T) {
	e := NewExporter(filepath.Join(t.TempDir(), "nonexistent"))
	s, err := e.Status(context.Background())
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if s.Status != "not_deployed" {
		t.Errorf("expected status 'not_deployed', got %q", s.Status)
	}
}

func TestStatus_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	e := NewExporter(dir)
	s, err := e.Status(context.Background())
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if s.Status != "not_deployed" {
		t.Errorf("expected status 'not_deployed', got %q", s.Status)
	}
}

func TestDestroy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExporter(dir)
	if err := e.Destroy(context.Background()); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory should have been removed")
	}
}
