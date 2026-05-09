package game

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/devrecon/ludus/internal/config"
)

// resolveServerBuildArgs assembles the UAT arguments for BuildCookRun.
// Returns the args slice, resolved output directory, and server target name.
func (b *Builder) resolveServerBuildArgs(projectPath string) ([]string, string, string, error) {
	arch := config.NormalizeArch(b.opts.Arch)
	outputDir := b.serverOutputDir(filepath.Dir(projectPath))
	serverTarget := b.serverTargetName()

	args := b.baseServerBuildArgs(projectPath, outputDir, serverTarget, arch)
	args = append(args, b.optionalServerBuildArgs()...)
	args = b.appendMaxJobsArg(args)

	return args, outputDir, serverTarget, nil
}

func (b *Builder) baseServerBuildArgs(projectPath, outputDir, serverTarget, arch string) []string {
	return []string{
		"BuildCookRun",
		fmt.Sprintf(`-project="%s"`, projectPath),
		"-platform=" + config.UEPlatformName(arch),
		"-server",
		"-noclient",
		fmt.Sprintf("-servertargetname=%s", serverTarget),
		"-build",
		"-stage",
		"-package",
		"-archive",
		fmt.Sprintf(`-archivedirectory="%s"`, outputDir),
	}
}

func (b *Builder) optionalServerBuildArgs() []string {
	args := b.serverConfigArgs()
	args = append(args, b.cookArgs()...)
	args = append(args, b.serverMapArgs()...)
	return args
}

func (b *Builder) serverConfigArgs() []string {
	if b.opts.ServerConfig == "" {
		return nil
	}
	return []string{fmt.Sprintf("-serverconfig=%s", b.opts.ServerConfig)}
}

func (b *Builder) cookArgs() []string {
	if b.opts.SkipCook {
		return []string{"-skipcook"}
	}
	return []string{"-cook"}
}

func (b *Builder) serverMapArgs() []string {
	if b.opts.ServerMap == "" {
		return nil
	}
	return []string{fmt.Sprintf(`-map="%s"`, b.opts.ServerMap)}
}

func (b *Builder) appendMaxJobsArg(args []string) []string {
	if jobs := b.resolveMaxJobs(runtime.GOOS == "windows"); jobs > 0 {
		fmt.Printf("  Limiting parallel compile actions to %d\n", jobs)
		return append(args, fmt.Sprintf("-MaxParallelActions=%d", jobs))
	}
	return args
}

func (b *Builder) serverOutputDir(projectDir string) string {
	if b.opts.OutputDir != "" {
		return b.opts.OutputDir
	}
	return filepath.Join(projectDir, "PackagedServer")
}

func (b *Builder) serverTargetName() string {
	if b.opts.ServerTarget != "" {
		return b.opts.ServerTarget
	}
	return b.opts.ProjectName + "Server"
}
