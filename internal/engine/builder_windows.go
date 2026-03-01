//go:build windows

package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// runBat executes a batch file with arguments, constructing the command line
// manually via SysProcAttr.CmdLine. This is necessary because cmd.exe's /c
// flag has quirky quote-stripping behavior: when the command line has more than
// two quote characters (e.g. a path with spaces AND a quoted argument like
// -CompilerArguments="/wd4756"), cmd strips the first and last quote from the
// entire line, breaking the path. The workaround is double-quoting: wrap the
// whole command in an extra pair of quotes so that after stripping, the inner
// quotes around the batch file path remain intact.
func (b *Builder) runBat(ctx context.Context, batPath string, args ...string) error {
	// Inner command: "path\to\file.bat" arg1 arg2 ...
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, `"`+batPath+`"`)
	parts = append(parts, args...)
	innerCmd := strings.Join(parts, " ")

	// Double-quote for cmd /c: cmd /c ""path\to\file.bat" arg1 arg2 ..."
	// cmd strips the outer quotes, leaving the inner ones intact.
	cmdLine := `cmd /c "` + innerCmd + `"`

	if b.Runner.Verbose || b.Runner.DryRun {
		fmt.Fprintf(b.Runner.Stdout, "+ cd %s && %s\n", b.opts.SourcePath, cmdLine)
	}
	if b.Runner.DryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, "cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdLine}
	cmd.Dir = b.opts.SourcePath
	cmd.Stdout = b.Runner.Stdout
	cmd.Stderr = b.Runner.Stderr
	return cmd.Run()
}

// Setup runs Setup.bat to download engine dependencies.
func (b *Builder) Setup(ctx context.Context) error {
	setupPath := filepath.Join(b.opts.SourcePath, "Setup.bat")
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		return fmt.Errorf("Setup.bat not found at %s", setupPath)
	}

	return b.runBat(ctx, setupPath)
}

// GenerateProjectFiles runs GenerateProjectFiles.bat.
func (b *Builder) GenerateProjectFiles(ctx context.Context) error {
	genPath := filepath.Join(b.opts.SourcePath, "GenerateProjectFiles.bat")
	if _, err := os.Stat(genPath); os.IsNotExist(err) {
		return fmt.Errorf("GenerateProjectFiles.bat not found at %s", genPath)
	}

	return b.runBat(ctx, genPath)
}

// compile builds ShaderCompileWorker and UnrealEditor using Build.bat.
// On Windows, Build.bat invokes UnrealBuildTool which manages parallelism
// internally. The -MaxParallelActions flag controls the number of concurrent
// compile actions. The /wd4756 suppresses C4756 (overflow in constant
// arithmetic) which MSVC 14.38 raises in experimental plugins like
// AnimNextAnimGraph and NNERuntimeRDG; UE5's -WarningsAsErrors would
// otherwise turn these into build failures.
func (b *Builder) compile(ctx context.Context, jobs int) error {
	buildBat := filepath.Join(b.opts.SourcePath, "Engine", "Build", "BatchFiles", "Build.bat")
	if _, err := os.Stat(buildBat); os.IsNotExist(err) {
		return fmt.Errorf("Build.bat not found at %s", buildBat)
	}

	targets := []string{"ShaderCompileWorker", "UnrealEditor"}
	for _, target := range targets {
		fmt.Printf("  Building %s...\n", target)
		err := b.runBat(ctx, buildBat,
			target,
			"Win64",
			"Development",
			"-WaitMutex",
			fmt.Sprintf("-MaxParallelActions=%d", jobs),
			`-CompilerArguments="/wd4756"`,
		)
		if err != nil {
			return fmt.Errorf("build %s failed: %w", target, err)
		}
	}
	return nil
}

type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

// autoDetectJobs calculates the number of parallel compile jobs based on
// available RAM. UE5 linking can spike ~8GB per job.
func autoDetectJobs() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GlobalMemoryStatusEx")

	var mem memoryStatusEx
	mem.length = uint32(unsafe.Sizeof(mem))

	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(&mem)))
	if ret == 0 {
		return 1
	}

	memGB := mem.totalPhys / (1024 * 1024 * 1024)
	jobs := max(int(memGB/8), 1)
	return jobs
}
