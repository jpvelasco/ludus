package binary

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/ludus/internal/deploy"
)

// Exporter copies the cooked server build to a local directory.
type Exporter struct {
	outputDir    string
	outputWriter io.Writer
}

// NewExporter creates a new binary Exporter targeting the given output directory.
func NewExporter(outputDir string) *Exporter {
	return &Exporter{outputDir: outputDir}
}

// SetOutput routes this exporter's status prints to w instead of the process
// stdout. The MCP destroy sweep calls it with its per-operation capture so
// the tool response carries the teardown transcript; CLI callers never call
// it (#765). Implements deploy.OutputSetter.
func (e *Exporter) SetOutput(w io.Writer) { e.outputWriter = w }

// output returns the writer status prints go to: the routed capture when
// SetOutput was called, else the process stdout.
func (e *Exporter) output() io.Writer {
	if e.outputWriter != nil {
		return e.outputWriter
	}
	return os.Stdout
}

func (e *Exporter) Name() string { return "binary" }

func (e *Exporter) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{
		NeedsContainerBuild: false,
		NeedsContainerPush:  false,
		NeedsAWS:            false,
		NeedsImageURI:       false,
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

	if err := checkExportOverlap(input.ServerBuildDir, absOut); err != nil {
		return nil, err
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

// checkExportOverlap rejects unsafe source/output directory pairs before any
// write. Exporting onto the source tree truncates files in place (copyFile
// opens the source, then os.Create truncates the same inode before io.Copy
// reads it), and an output directory inside the source walks over its own
// growing copy. Both directions are rejected, including aliases created by
// symlinks, so only disjoint (sibling) directories are accepted.
func checkExportOverlap(src, dst string) error {
	srcReal, err := realPath(src)
	if err != nil {
		return fmt.Errorf("resolving server build directory: %w", err)
	}
	dstReal, err := realPath(dst)
	if err != nil {
		return fmt.Errorf("resolving output directory: %w", err)
	}

	switch {
	case srcReal == dstReal:
		return fmt.Errorf("output directory %s is the server build directory %s; refusing to export onto the source", dst, src)
	case containsPath(srcReal, dstReal):
		return fmt.Errorf("output directory %s is inside the server build directory %s; export to a sibling directory instead", dst, src)
	case containsPath(dstReal, srcReal):
		return fmt.Errorf("server build directory %s is inside the output directory %s; export to a sibling directory instead", src, dst)
	}
	return nil
}

// realPath returns the canonical absolute path with symlinks resolved. A path
// that does not exist yet (the output directory is created later) is
// canonicalized through its nearest existing ancestor, so symlinked parents
// still alias correctly.
func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs, nil
	}
	resolvedParent, err := realPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(abs)), nil
}

// containsPath reports whether child is dir itself or a descendant of it.
// Both paths must be absolute and clean.
func containsPath(dir, child string) bool {
	rel, err := filepath.Rel(dir, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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

	fmt.Fprintf(e.output(), "Removed %s\n", absOut)
	return nil
}
