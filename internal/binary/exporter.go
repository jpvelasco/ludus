package binary

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/devrecon/ludus/internal/deploy"
)

// Exporter copies the cooked server build to a local directory.
type Exporter struct {
	outputDir string
}

// NewExporter creates a new binary Exporter targeting the given output directory.
func NewExporter(outputDir string) *Exporter {
	return &Exporter{outputDir: outputDir}
}

func (e *Exporter) Name() string { return "binary" }

func (e *Exporter) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{
		NeedsContainerBuild: false,
		NeedsContainerPush:  false,
		SupportsSession:     false,
		SupportsDeploy:      true,
		SupportsDestroy:     true,
	}
}

func (e *Exporter) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	if input.ServerBuildDir == "" {
		return nil, fmt.Errorf("server build directory not set")
	}

	if _, err := os.Stat(input.ServerBuildDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("server build directory does not exist: %s", input.ServerBuildDir)
	}

	absOut, err := filepath.Abs(e.outputDir)
	if err != nil {
		return nil, fmt.Errorf("resolving output path: %w", err)
	}

	if err := os.MkdirAll(absOut, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	if err := copyDir(input.ServerBuildDir, absOut); err != nil {
		return nil, fmt.Errorf("copying server build: %w", err)
	}

	return &deploy.DeployResult{
		TargetName: "binary",
		Status:     "exported",
		Detail:     absOut,
	}, nil
}

func (e *Exporter) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	absOut, err := filepath.Abs(e.outputDir)
	if err != nil {
		return &deploy.DeployStatus{
			TargetName: "binary",
			Status:     "error",
			Detail:     err.Error(),
		}, nil
	}

	entries, err := os.ReadDir(absOut)
	if err != nil {
		if os.IsNotExist(err) {
			return &deploy.DeployStatus{
				TargetName: "binary",
				Status:     "not_deployed",
				Detail:     "output directory does not exist",
			}, nil
		}
		return &deploy.DeployStatus{
			TargetName: "binary",
			Status:     "error",
			Detail:     err.Error(),
		}, nil
	}

	if len(entries) == 0 {
		return &deploy.DeployStatus{
			TargetName: "binary",
			Status:     "not_deployed",
			Detail:     "output directory is empty",
		}, nil
	}

	return &deploy.DeployStatus{
		TargetName: "binary",
		Status:     "active",
		Detail:     absOut,
	}, nil
}

// copyDir recursively copies src to dst, preserving file permissions.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) (retErr error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

func (e *Exporter) Destroy(ctx context.Context) error {
	absOut, err := filepath.Abs(e.outputDir)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}

	if err := os.RemoveAll(absOut); err != nil {
		return fmt.Errorf("removing output directory: %w", err)
	}

	fmt.Printf("Removed %s\n", absOut)
	return nil
}
