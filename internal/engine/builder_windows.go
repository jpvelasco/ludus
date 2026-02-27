//go:build windows

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Setup runs Setup.bat to download engine dependencies.
func (b *Builder) Setup(ctx context.Context) error {
	setupPath := filepath.Join(b.opts.SourcePath, "Setup.bat")
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		return fmt.Errorf("Setup.bat not found at %s", setupPath)
	}

	return b.Runner.RunInDir(ctx, b.opts.SourcePath, "cmd", "/c", "Setup.bat")
}

// GenerateProjectFiles runs GenerateProjectFiles.bat.
func (b *Builder) GenerateProjectFiles(ctx context.Context) error {
	genPath := filepath.Join(b.opts.SourcePath, "GenerateProjectFiles.bat")
	if _, err := os.Stat(genPath); os.IsNotExist(err) {
		return fmt.Errorf("GenerateProjectFiles.bat not found at %s", genPath)
	}

	return b.Runner.RunInDir(ctx, b.opts.SourcePath, "cmd", "/c", "GenerateProjectFiles.bat")
}

// compile builds ShaderCompileWorker and UnrealEditor using Build.bat.
// On Windows, Build.bat invokes UnrealBuildTool which manages parallelism
// internally. The -MaxParallelActions flag controls the number of concurrent
// compile actions. The /wd4756 suppresses C4756 (overflow in constant
// arithmetic) which MSVC 14.38 raises in experimental plugins like
// AnimNextAnimGraph and NNERuntimeRDG; UE5's -WarningsAsErrors would
// otherwise turn these into build failures.
func (b *Builder) compile(ctx context.Context, jobs int) error {
	buildBat := filepath.Join("Engine", "Build", "BatchFiles", "Build.bat")
	absCheck := filepath.Join(b.opts.SourcePath, buildBat)
	if _, err := os.Stat(absCheck); os.IsNotExist(err) {
		return fmt.Errorf("Build.bat not found at %s", absCheck)
	}

	targets := []string{"ShaderCompileWorker", "UnrealEditor"}
	for _, target := range targets {
		fmt.Printf("  Building %s...\n", target)
		args := []string{
			"/c", buildBat,
			target,
			"Win64",
			"Development",
			"-WaitMutex",
			fmt.Sprintf("-MaxParallelActions=%d", jobs),
			`-CompilerArguments="/wd4756"`,
		}
		if err := b.Runner.RunInDir(ctx, b.opts.SourcePath, "cmd", args...); err != nil {
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
