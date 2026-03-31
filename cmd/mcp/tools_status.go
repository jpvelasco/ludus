package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/dflint"
	"github.com/devrecon/ludus/internal/dockerbuild"
	internalstatus "github.com/devrecon/ludus/internal/status"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type statusInput struct{}

type securityFinding struct {
	Source  string `json:"source"`
	Rule    string `json:"rule"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

type securityScan struct {
	Target   string            `json:"target"`
	Summary  string            `json:"summary"`
	Findings []securityFinding `json:"findings,omitempty"`
}

type statusResult struct {
	Stages   []internalstatus.StageStatus `json:"stages"`
	Security []securityScan               `json:"security,omitempty"`
}

func registerStatusTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_status",
		Description: "Check status of all pipeline stages (engine, game, container, deploy, session) and run security scans (hadolint Dockerfile lint + trivy image vulnerability scan).",
	}, handleStatus)
}

func handleStatus(ctx context.Context, _ *mcp.CallToolRequest, _ statusInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	target, err := globals.ResolveTarget(ctx, cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve deploy target: %v\n", err)
	}

	stages := internalstatus.CheckAll(ctx, cfg, target)
	security := runSecurityScans(cfg)

	return resultOK(statusResult{Stages: stages, Security: security})
}

func runSecurityScans(cfg *config.Config) []securityScan {
	var scans []securityScan

	// Lint game server Dockerfile
	gameBuilder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ServerPort:   cfg.Container.ServerPort,
		ProjectName:  cfg.Game.ProjectName,
		ServerTarget: cfg.Game.ResolvedServerTarget(),
		Arch:         cfg.Game.ResolvedArch(),
	}, nil)
	gameResult := dflint.LintDockerfile(gameBuilder.GenerateDockerfile())
	scans = append(scans, lintResultToScan("Game Dockerfile", gameResult))

	// Lint engine Dockerfile
	engineResult := dflint.LintDockerfile(dockerbuild.GenerateEngineDockerfile(dockerbuild.DockerfileOptions{
		MaxJobs:   cfg.Engine.MaxJobs,
		BaseImage: cfg.Engine.DockerBaseImage,
	}))
	scans = append(scans, lintResultToScan("Engine Dockerfile", engineResult))

	// Scan container image with trivy
	imageRef := fmt.Sprintf("%s:%s", cfg.Container.ImageName, cfg.Container.Tag)
	imageResult := dflint.LintImage(imageRef)
	scans = append(scans, lintResultToScan("Container Image ("+imageRef+")", imageResult))

	return scans
}

func lintResultToScan(target string, result *dflint.LintResult) securityScan {
	scan := securityScan{
		Target:  target,
		Summary: result.Summary(),
	}
	for _, f := range result.Findings {
		scan.Findings = append(scan.Findings, securityFinding{
			Source:  f.Source,
			Rule:    f.Rule,
			Level:   string(f.Level),
			Message: f.Message,
			Line:    f.Line,
		})
	}
	return scan
}
