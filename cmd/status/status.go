package status

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/spf13/cobra"
)

type stageStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Cmd is the status command.
var Cmd = &cobra.Command{
	Use:   "status",
	Short: "Check status of all pipeline stages",
	Long: `Displays the current state of each pipeline stage:

  - Engine:    Is the engine source present? Built?
  - Lyra:      Is the server target compiled? Content cooked?
  - Container: Is the Docker image built? Pushed to ECR?
  - GameLift:  Is the fleet deployed? Active? Game sessions?`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg
	var stages []stageStatus

	// 1. Engine source
	stages = append(stages, checkEngineSource(cfg.Engine.SourcePath))

	// 2. Engine build
	stages = append(stages, checkEngineBuild(cfg.Engine.SourcePath))

	// 3. Lyra server build
	lyraOutputDir := ""
	if cfg.Lyra.ProjectPath != "" {
		lyraOutputDir = filepath.Join(filepath.Dir(cfg.Lyra.ProjectPath), "PackagedServer")
	} else if cfg.Engine.SourcePath != "" {
		lyraOutputDir = filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer")
	}
	stages = append(stages, checkLyraBuild(lyraOutputDir))

	// 4. Container image
	stages = append(stages, checkContainerImage(cfg.Container.ImageName))

	// 5. GameLift fleet
	stages = append(stages, checkGameLiftFleet(cmd, cfg))

	if globals.JSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stages)
	}

	fmt.Println("Pipeline Status")
	fmt.Println("===============")
	for _, s := range stages {
		marker := "[--]"
		switch s.Status {
		case "ok":
			marker = "[OK]"
		case "fail":
			marker = "[FAIL]"
		}
		line := fmt.Sprintf("  %s  %-24s", marker, s.Name)
		if s.Detail != "" {
			line += "  " + s.Detail
		}
		fmt.Println(line)
	}
	fmt.Println()
	return nil
}

func checkEngineSource(sourcePath string) stageStatus {
	s := stageStatus{Name: "Engine Source"}
	if sourcePath == "" {
		s.Status = "fail"
		s.Detail = "not configured"
		return s
	}
	setupPath := filepath.Join(sourcePath, "Setup.sh")
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		s.Status = "fail"
		s.Detail = "Setup.sh not found"
		return s
	}
	s.Status = "ok"
	s.Detail = sourcePath
	return s
}

func checkEngineBuild(sourcePath string) stageStatus {
	s := stageStatus{Name: "Engine Build"}
	if sourcePath == "" {
		s.Status = "unknown"
		s.Detail = "source path not configured"
		return s
	}
	editorPath := filepath.Join(sourcePath, "Engine", "Binaries", "Linux", "UnrealEditor")
	if _, err := os.Stat(editorPath); os.IsNotExist(err) {
		s.Status = "fail"
		s.Detail = "UnrealEditor binary not found"
		return s
	}
	s.Status = "ok"
	s.Detail = "UnrealEditor binary found"
	return s
}

func checkLyraBuild(outputDir string) stageStatus {
	s := stageStatus{Name: "Lyra Server Build"}
	if outputDir == "" {
		s.Status = "unknown"
		s.Detail = "output directory unknown"
		return s
	}
	serverDir := filepath.Join(outputDir, "LinuxServer")
	if _, err := os.Stat(serverDir); os.IsNotExist(err) {
		s.Status = "fail"
		s.Detail = "not built"
		return s
	}
	s.Status = "ok"
	s.Detail = serverDir
	return s
}

func checkContainerImage(imageName string) stageStatus {
	s := stageStatus{Name: "Container Image"}
	if imageName == "" {
		s.Status = "unknown"
		s.Detail = "image name not configured"
		return s
	}

	out, err := exec.Command("docker", "images", imageName, "--format", "{{.Tag}}").Output()
	if err != nil {
		s.Status = "unknown"
		s.Detail = "docker not available"
		return s
	}

	tags := strings.TrimSpace(string(out))
	if tags == "" {
		s.Status = "fail"
		s.Detail = "no image found"
		return s
	}
	s.Status = "ok"
	s.Detail = fmt.Sprintf("tags: %s", strings.ReplaceAll(tags, "\n", ", "))
	return s
}

func checkGameLiftFleet(cmd *cobra.Command, cfg *config.Config) stageStatus {
	s := stageStatus{Name: "GameLift Fleet"}

	if cfg.AWS.Region == "" {
		s.Status = "unknown"
		s.Detail = "AWS not configured"
		return s
	}

	awsCfg, err := gamelift.LoadAWSConfig(cmd.Context(), cfg.AWS.Region)
	if err != nil {
		s.Status = "unknown"
		s.Detail = "AWS config error"
		return s
	}

	deployer := gamelift.NewDeployer(gamelift.DeployOptions{
		Region:             cfg.AWS.Region,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
	}, awsCfg)

	fleetStatus, err := deployer.GetFleetStatus(cmd.Context())
	if err != nil {
		s.Status = "fail"
		s.Detail = "no fleet found"
		return s
	}

	s.Status = "ok"
	s.Detail = fmt.Sprintf("%s (%s)", fleetStatus.FleetID, fleetStatus.Status)
	return s
}
