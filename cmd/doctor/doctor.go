package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/dflint"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

// Cmd is the doctor command for comprehensive diagnostics.
var Cmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run comprehensive diagnostics on the Ludus environment",
	Long: `Performs deeper diagnostics beyond 'ludus init', including:

  - Stale DLLs or build artifacts from previous engine versions
  - Toolchain version mismatch (env vs registry vs engine requirement)
  - Disk space for upcoming builds
  - Partial or corrupted build state
  - AWS credential expiry
  - Build cache integrity
  - Common misconfigurations

Use this when something isn't working and 'ludus init' passes.`,
	RunE: runDoctor,
}

// diagnostic represents a single diagnostic check result.
type diagnostic struct {
	name    string
	status  string // "ok", "warn", "fail"
	message string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	fmt.Println("Running diagnostics...")
	fmt.Println()

	var checks []diagnostic

	checks = append(checks, checkToolchainConsistency(cfg))
	checks = append(checks, checkStaleBuildArtifacts(cfg))
	checks = append(checks, checkBuildState(cfg))
	checks = append(checks, checkCacheIntegrity())
	checks = append(checks, checkDiskSpace(cfg))
	checks = append(checks, checkAWSCredentialExpiry())
	checks = append(checks, checkDockerDaemon())
	checks = append(checks, checkDockerfileSecurity(cfg)...)
	checks = append(checks, checkGitState())

	fails := 0
	warns := 0
	for _, d := range checks {
		marker := "[OK]  "
		switch d.status {
		case "fail":
			marker = "[FAIL]"
			fails++
		case "warn":
			marker = "[WARN]"
			warns++
		}
		fmt.Printf("  %s %-30s %s\n", marker, d.name, d.message)
	}

	fmt.Println()
	if fails > 0 {
		fmt.Printf("%d issue(s) found", fails)
		if warns > 0 {
			fmt.Printf(", %d warning(s)", warns)
		}
		fmt.Println()
		return fmt.Errorf("%d diagnostic check(s) failed", fails)
	}
	if warns > 0 {
		fmt.Printf("No issues found (%d warning(s)).\n", warns)
	} else {
		fmt.Println("No issues found.")
	}
	return nil
}

// checkToolchainConsistency verifies the toolchain version in the environment
// matches what the engine expects.
func checkToolchainConsistency(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Toolchain Consistency"}

	if cfg.Engine.SourcePath == "" {
		d.status = "ok"
		d.message = "skipped — no engine source configured"
		return d
	}

	tc := toolchain.CheckToolchain(cfg.Engine.SourcePath, cfg.Engine.Version)
	if tc.Required == nil {
		d.status = "ok"
		d.message = "no known toolchain requirement for this engine version"
		return d
	}

	if !tc.Found {
		d.status = "warn"
		d.message = fmt.Sprintf("required %s not found; run 'ludus init --fix'", tc.Required.SDKVersion)
		return d
	}

	// Check if LINUX_MULTIARCH_ROOT points to the right version
	lmr := os.Getenv("LINUX_MULTIARCH_ROOT")
	if lmr != "" && !strings.Contains(lmr, tc.Required.SDKVersion) {
		d.status = "warn"
		d.message = fmt.Sprintf("LINUX_MULTIARCH_ROOT points to %s but engine requires %s; restart terminal after toolchain install",
			filepath.Base(lmr), tc.Required.SDKVersion)
		return d
	}

	d.status = "ok"
	d.message = fmt.Sprintf("%s found and matches engine requirement", tc.Required.SDKVersion)
	return d
}

// checkStaleBuildArtifacts looks for build artifacts that might be from a
// different engine version.
func checkStaleBuildArtifacts(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Build Artifacts"}

	if cfg.Engine.SourcePath == "" {
		d.status = "ok"
		d.message = "skipped — no engine source configured"
		return d
	}

	// Check if UnrealEditor exists but is very old (> 30 days)
	var editorPath string
	if runtime.GOOS == "windows" {
		editorPath = filepath.Join(cfg.Engine.SourcePath, "Engine", "Binaries", "Win64", "UnrealEditor.exe")
	} else {
		editorPath = filepath.Join(cfg.Engine.SourcePath, "Engine", "Binaries", "Linux", "UnrealEditor")
	}

	info, err := os.Stat(editorPath)
	if err != nil {
		d.status = "ok"
		d.message = "no engine build found (clean state)"
		return d
	}

	age := time.Since(info.ModTime())
	if age > 30*24*time.Hour {
		d.status = "warn"
		d.message = fmt.Sprintf("engine build is %d days old; consider rebuilding", int(age.Hours()/24))
		return d
	}

	d.status = "ok"
	d.message = fmt.Sprintf("engine build is %d days old", int(age.Hours()/24))
	return d
}

// checkBuildState verifies state.json consistency — checks if referenced
// files and directories still exist.
func checkBuildState(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Build State"}

	st, err := state.Load()
	if err != nil {
		d.status = "warn"
		d.message = "could not read .ludus/state.json"
		return d
	}

	var issues []string

	// Check client binary exists
	if st.Client != nil && st.Client.BinaryPath != "" {
		if _, err := os.Stat(st.Client.BinaryPath); os.IsNotExist(err) {
			issues = append(issues, "client binary missing: "+st.Client.BinaryPath)
		}
	}

	// Check deploy state references
	if st.Deploy != nil && st.Deploy.Status == "active" {
		// Deployment marked active — check if fleet still exists in state
		if st.Fleet == nil && st.EC2Fleet == nil && st.Anywhere == nil {
			issues = append(issues, "deploy marked active but no fleet state found")
		}
	}

	if len(issues) > 0 {
		d.status = "warn"
		d.message = strings.Join(issues, "; ")
		return d
	}

	d.status = "ok"
	d.message = "state references are consistent"
	return d
}

// checkCacheIntegrity verifies the build cache is readable.
func checkCacheIntegrity() diagnostic {
	d := diagnostic{name: "Build Cache"}

	c, err := cache.Load()
	if err != nil {
		d.status = "warn"
		d.message = fmt.Sprintf("cache unreadable: %v; builds will re-run from scratch", err)
		return d
	}

	_ = c // cache loaded successfully — that's all we need to verify

	d.status = "ok"
	d.message = "cache file readable"
	return d
}

// checkDiskSpace warns if available disk space is low for builds.
func checkDiskSpace(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Disk Space"}

	// Get the path to check — engine source or current directory
	checkPath := cfg.Engine.SourcePath
	if checkPath == "" {
		var err error
		checkPath, err = os.Getwd()
		if err != nil {
			d.status = "ok"
			d.message = "could not determine path to check"
			return d
		}
	}

	freeGB := getFreeDiskGB(checkPath)
	if freeGB < 0 {
		d.status = "ok"
		d.message = "could not determine free space"
		return d
	}

	if freeGB < 50 {
		d.status = "fail"
		d.message = fmt.Sprintf("%.0f GB free — builds require 50-100 GB free space", freeGB)
		return d
	}
	if freeGB < 100 {
		d.status = "warn"
		d.message = fmt.Sprintf("%.0f GB free — consider freeing space (100 GB recommended for builds)", freeGB)
		return d
	}

	d.status = "ok"
	d.message = fmt.Sprintf("%.0f GB free", freeGB)
	return d
}

// checkAWSCredentialExpiry checks if AWS credentials are configured and valid.
func checkAWSCredentialExpiry() diagnostic {
	d := diagnostic{name: "AWS Credentials"}

	if _, err := exec.LookPath("aws"); err != nil {
		d.status = "ok"
		d.message = "skipped — AWS CLI not installed"
		return d
	}

	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "json")
	if err := cmd.Run(); err != nil {
		d.status = "warn"
		d.message = "credentials expired or not configured; run 'aws configure' or 'aws sso login'"
		return d
	}

	d.status = "ok"
	d.message = "credentials valid"
	return d
}

// checkDockerDaemon checks if Docker is running (not just installed).
func checkDockerDaemon() diagnostic {
	d := diagnostic{name: "Docker Daemon"}

	if _, err := exec.LookPath("docker"); err != nil {
		if runtime.GOOS == "windows" {
			d.status = "ok"
			d.message = "skipped — not needed for Windows client workflow"
		} else {
			d.status = "warn"
			d.message = "docker not installed"
		}
		return d
	}

	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		d.status = "warn"
		d.message = "docker installed but daemon not running; start Docker Desktop or 'sudo systemctl start docker'"
		return d
	}

	d.status = "ok"
	d.message = "docker daemon running"
	return d
}

// checkDockerfileSecurity lints generated Dockerfiles and optionally scans container images.
func checkDockerfileSecurity(cfg *config.Config) []diagnostic {
	var checks []diagnostic

	// Lint game server Dockerfile
	gameBuilder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ServerPort:   cfg.Container.ServerPort,
		ProjectName:  cfg.Game.ProjectName,
		ServerTarget: cfg.Game.ResolvedServerTarget(),
		Arch:         cfg.Game.ResolvedArch(),
	}, nil)
	gameDF := gameBuilder.GenerateDockerfile()
	gameResult := dflint.LintDockerfile(gameDF)

	gameDiag := diagnostic{name: "Game Dockerfile"}
	gameDiag.status = "ok"
	gameDiag.message = gameResult.Summary()
	if !gameResult.HadolintAvailable {
		gameDiag.message += "; install hadolint for extended checks"
	}
	if gameResult.HasWarnings() {
		gameDiag.status = "warn"
	}
	checks = append(checks, gameDiag)

	// Lint engine Dockerfile
	engineDF := dockerbuild.GenerateEngineDockerfile(dockerbuild.DockerfileOptions{
		MaxJobs:   cfg.Engine.MaxJobs,
		BaseImage: cfg.Engine.DockerBaseImage,
	})
	engineResult := dflint.LintDockerfile(engineDF)

	// Downgrade no-root-user to info for engine build containers (expected)
	for i := range engineResult.Findings {
		if engineResult.Findings[i].Rule == "no-root-user" {
			engineResult.Findings[i].Level = dflint.SeverityInfo
		}
	}

	engineDiag := diagnostic{name: "Engine Dockerfile"}
	engineDiag.status = "ok"
	engineDiag.message = engineResult.Summary()
	if !engineResult.HadolintAvailable {
		engineDiag.message += "; install hadolint for extended checks"
	}
	if engineResult.HasWarnings() {
		engineDiag.status = "warn"
	}
	checks = append(checks, engineDiag)

	// Scan container image with trivy (if available and image exists)
	imageRef := fmt.Sprintf("%s:%s", cfg.Container.ImageName, cfg.Container.Tag)
	imageResult := dflint.LintImage(imageRef)

	imageDiag := diagnostic{name: "Container Image"}
	switch {
	case !imageResult.TrivyAvailable:
		imageDiag.status = "ok"
		imageDiag.message = "skipped — install trivy for vulnerability scanning"
	case len(imageResult.Findings) == 0:
		imageDiag.status = "ok"
		imageDiag.message = fmt.Sprintf("no HIGH/CRITICAL vulnerabilities in %s", imageRef)
	default:
		imageDiag.status = "warn"
		imageDiag.message = imageResult.Summary()
	}
	checks = append(checks, imageDiag)

	return checks
}

// checkGitState checks for uncommitted changes in the engine source (which
// can cause UBT to rebuild everything).
func checkGitState() diagnostic {
	d := diagnostic{name: "Git Status"}

	if _, err := exec.LookPath("git"); err != nil {
		d.status = "ok"
		d.message = "git not available"
		return d
	}

	cmd := exec.Command("git", "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		d.status = "ok"
		d.message = "not in a git repository"
		return d
	}

	lines := strings.TrimSpace(string(out))
	if lines == "" {
		d.status = "ok"
		d.message = "working tree clean"
		return d
	}

	count := len(strings.Split(lines, "\n"))
	d.status = "ok"
	d.message = fmt.Sprintf("%d modified file(s) in working tree", count)
	return d
}
