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
	details []string // optional per-finding detail lines
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	fmt.Println("Running diagnostics...")
	fmt.Println()

	var checks []diagnostic

	checks = append(checks, checkToolchainConsistency(cfg))
	checks = append(checks, checkStaleBuildArtifacts(cfg))
	checks = append(checks, checkBuildState())
	checks = append(checks, checkCacheIntegrity())
	checks = append(checks, checkDiskSpace(cfg))
	checks = append(checks, checkAWSCredentialExpiry())
	checks = append(checks, checkDockerDaemon())
	checks = append(checks, checkDockerfileSecurity(cfg)...)
	checks = append(checks, checkGitState())

	return printDiagnostics(checks)
}

// printDiagnostics formats and prints diagnostic results, returning an error if any checks failed.
func printDiagnostics(checks []diagnostic) error {
	fails, warns := countDiagnostics(checks)

	for _, d := range checks {
		marker := diagnosticMarker(d.status)
		fmt.Printf("  %s %-30s %s\n", marker, d.name, d.message)
		for _, detail := range d.details {
			fmt.Printf("         %-30s   %s\n", "", detail)
		}
	}

	fmt.Println()
	return formatDiagnosticSummary(fails, warns)
}

// countDiagnostics counts failures and warnings in the check results.
func countDiagnostics(checks []diagnostic) (fails, warns int) {
	for _, d := range checks {
		switch d.status {
		case "fail":
			fails++
		case "warn":
			warns++
		}
	}
	return
}

// diagnosticMarker returns the display marker for a diagnostic status.
func diagnosticMarker(status string) string {
	switch status {
	case "fail":
		return "[FAIL]"
	case "warn":
		return "[WARN]"
	default:
		return "[OK]  "
	}
}

// formatDiagnosticSummary prints the summary line and returns an error if any checks failed.
func formatDiagnosticSummary(fails, warns int) error {
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
// clientBinaryIssue returns a warning if the client binary path is set but the file is missing.
func clientBinaryIssue(st *state.State) string {
	if st.Client == nil || st.Client.BinaryPath == "" {
		return ""
	}
	if _, err := os.Stat(st.Client.BinaryPath); err != nil {
		if os.IsNotExist(err) {
			return "client binary missing: " + st.Client.BinaryPath
		}
		return fmt.Sprintf("client binary error: %v", err)
	}
	return ""
}

// fleetStateIssue returns a warning if deploy is active but no fleet state exists.
func fleetStateIssue(st *state.State) string {
	if st.Deploy == nil || st.Deploy.Status != "active" {
		return ""
	}
	if st.Fleet != nil || st.EC2Fleet != nil || st.Anywhere != nil {
		return ""
	}
	return "deploy marked active but no fleet state found"
}

func checkBuildState() diagnostic {
	st, err := state.Load()
	if err != nil {
		return diagnostic{name: "Build State", status: "warn", message: "could not read .ludus/state.json"}
	}

	var issues []string
	if issue := clientBinaryIssue(st); issue != "" {
		issues = append(issues, issue)
	}
	if issue := fleetStateIssue(st); issue != "" {
		issues = append(issues, issue)
	}

	if len(issues) > 0 {
		return diagnostic{name: "Build State", status: "warn", message: strings.Join(issues, "; ")}
	}
	return diagnostic{name: "Build State", status: "ok", message: "state references are consistent"}
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
// This specifically checks Docker (not Podman) because GameLift container
// builds and ECR pushes currently require Docker. Engine/game builds support
// both Docker and Podman via --backend.
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
	gameResult := dflint.LintDockerfile(gameBuilder.GenerateDockerfile())
	checks = append(checks, lintResultToDiagnostic("Game Dockerfile", gameResult))

	// Lint engine Dockerfile
	engineDF := dockerbuild.GenerateEngineDockerfile(dockerbuild.DockerfileOptions{
		MaxJobs:   cfg.Engine.MaxJobs,
		BaseImage: cfg.Engine.DockerBaseImage,
	})
	engineResult := dflint.LintDockerfile(engineDF)
	downgradeRule(engineResult, "no-root-user", dflint.SeverityInfo)
	checks = append(checks, lintResultToDiagnostic("Engine Dockerfile", engineResult))

	// Scan container image with trivy
	checks = append(checks, checkContainerImage(cfg))

	return checks
}

// lintResultToDiagnostic converts a LintResult into a diagnostic.
func lintResultToDiagnostic(name string, result *dflint.LintResult) diagnostic {
	d := diagnostic{name: name, status: "ok"}
	d.message = result.Summary()
	d.details = result.FindingsDetail()
	if !result.HadolintAvailable {
		d.message += "; install hadolint for extended checks"
	}
	if result.HasErrors() {
		d.status = "fail"
	} else if result.HasWarnings() {
		d.status = "warn"
	}
	return d
}

// downgradeRule changes all findings matching rule to the given severity.
func downgradeRule(result *dflint.LintResult, rule string, level dflint.Severity) {
	for i := range result.Findings {
		if result.Findings[i].Rule == rule {
			result.Findings[i].Level = level
		}
	}
}

// checkContainerImage scans the container image for vulnerabilities.
func checkContainerImage(cfg *config.Config) diagnostic {
	imageRef := fmt.Sprintf("%s:%s", cfg.Container.ImageName, cfg.Container.Tag)
	result := dflint.LintImage(imageRef)

	d := diagnostic{name: "Container Image"}
	switch {
	case !result.TrivyAvailable:
		d.status = "ok"
		d.message = "skipped — install trivy for vulnerability scanning"
	case len(result.Findings) == 0:
		d.status = "ok"
		d.message = fmt.Sprintf("no HIGH/CRITICAL vulnerabilities in %s", imageRef)
	default:
		d.status = "warn"
		d.message = result.Summary()
		d.details = result.FindingsDetail()
	}
	return d
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
