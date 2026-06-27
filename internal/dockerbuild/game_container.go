package dockerbuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// prepareBuildContext resolves the output directory to an absolute path and
// creates it if needed. defaultSubdir is used when OutputDir is empty.
func (b *DockerGameBuilder) prepareBuildContext(outputDir, defaultSubdir string) (string, error) {
	if outputDir == "" {
		outputDir = filepath.Join(".", defaultSubdir)
	}
	if !filepath.IsAbs(outputDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting working directory: %w", err)
		}
		outputDir = filepath.Join(cwd, outputDir)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}
	return outputDir, nil
}

// runServerBuildContainer writes the build script and runs the Docker container.
func (b *DockerGameBuilder) runServerBuildContainer(ctx context.Context, outputDir string) error {
	buildScript := b.generateBuildScript(true)
	cli := ContainerCLI(b.opts.Runtime)
	return b.runBuildContainer(ctx, outputDir, buildScript, cli+" game build")
}

// runClientBuildContainer writes the client build script and runs the Docker container.
func (b *DockerGameBuilder) runClientBuildContainer(ctx context.Context, outputDir string) error {
	buildScript := b.generateBuildScript(false)
	cli := ContainerCLI(b.opts.Runtime)
	return b.runBuildContainer(ctx, outputDir, buildScript, cli+" client build")
}

// runBuildContainer mounts a preamble script and a build script into a container.
// The preamble runs as root (installs deps, creates non-root user), then re-execs
// the build script as the ue user via su -p.
func (b *DockerGameBuilder) runBuildContainer(ctx context.Context, outputDir, script, label string) error {
	preamble := b.scriptPreamble()

	preambleFile, err := os.CreateTemp("", "ludus-preamble-*.sh")
	if err != nil {
		return fmt.Errorf("creating temp preamble script: %w", err)
	}
	defer os.Remove(preambleFile.Name())

	if _, err := preambleFile.WriteString(preamble); err != nil {
		preambleFile.Close()
		return fmt.Errorf("writing preamble script: %w", err)
	}
	preambleFile.Close()
	if err := os.Chmod(preambleFile.Name(), 0644); err != nil { //nolint:gosec // 0644 intentional: container non-root user must read this file
		return fmt.Errorf("chmod preamble script: %w", err)
	}

	buildFile, err := os.CreateTemp("", "ludus-build-*.sh")
	if err != nil {
		return fmt.Errorf("creating temp build script: %w", err)
	}
	defer os.Remove(buildFile.Name())

	if _, err := buildFile.WriteString(script); err != nil {
		buildFile.Close()
		return fmt.Errorf("writing build script: %w", err)
	}
	buildFile.Close()
	if err := os.Chmod(buildFile.Name(), 0644); err != nil { //nolint:gosec // 0644 intentional: container non-root user must read this file
		return fmt.Errorf("chmod build script: %w", err)
	}

	args := []string{
		"run", "--rm",
		"--platform", "linux/amd64", // game builds run on forced amd64 engine image; arm64 is cross inside via UAT flags
		"-v", fmt.Sprintf("%s:/output", outputDir),
		"-v", fmt.Sprintf("%s:/preamble.sh:ro", preambleFile.Name()),
		"-v", fmt.Sprintf("%s:/build.sh:ro", buildFile.Name()),
	}

	if b.isExternalProject() {
		projectDir := filepath.Dir(b.opts.ProjectPath)
		args = append(args, "-v", fmt.Sprintf("%s:/project", projectDir))
	}

	ddcExtra, err := b.ddcArgs()
	if err != nil {
		return err
	}
	args = append(args, ddcExtra...)
	args = append(args, b.envArgs()...)

	args = append(args, b.opts.EngineImage, "bash", "/preamble.sh")

	cli := ContainerCLI(b.opts.Runtime)
	if err := b.Runner.Run(ctx, cli, args...); err != nil {
		return fmt.Errorf("%s failed: %w", label, err)
	}
	return nil
}
