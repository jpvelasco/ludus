package doctor

import (
	"fmt"
	"os/exec"

	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/dflint"
	"github.com/devrecon/ludus/internal/dockerbuild"
)

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
	if _, err := exec.LookPath("docker"); err != nil {
		return dockerNotInstalledDiagnostic()
	}

	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return diagnostic{
			name:    "Docker Daemon",
			status:  "warn",
			message: "docker installed but daemon not running; start Docker Desktop or 'sudo systemctl start docker'",
		}
	}

	return diagnostic{name: "Docker Daemon", status: "ok", message: "docker daemon running"}
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
