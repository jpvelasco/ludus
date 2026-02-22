package status

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/state"
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
  - Game:      Is the server target compiled? Content cooked?
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

	// 3. Game server build
	gameOutputDir := ""
	if cfg.Game.ProjectPath != "" {
		gameOutputDir = filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer")
	} else if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		gameOutputDir = filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer")
	}
	stages = append(stages, checkServerBuild(cfg.Game.ProjectName, gameOutputDir))

	// 4. Container image
	stages = append(stages, checkContainerImage(cfg.Container.ImageName))

	// 5. Game client build
	stages = append(stages, checkClientBuild(cfg.Game.ProjectName))

	// 6. GameLift fleet
	stages = append(stages, checkGameLiftFleet(cmd, cfg))

	// 7. Game session
	stages = append(stages, checkGameSession())

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
	// Check for the platform-appropriate setup script
	setupFile := "Setup.sh"
	if runtime.GOOS == "windows" {
		setupFile = "Setup.bat"
	}
	setupPath := filepath.Join(sourcePath, setupFile)
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		s.Status = "fail"
		s.Detail = setupFile + " not found"
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
	// Check for the platform-appropriate editor binary
	var editorPath string
	if runtime.GOOS == "windows" {
		editorPath = filepath.Join(sourcePath, "Engine", "Binaries", "Win64", "UnrealEditor.exe")
	} else {
		editorPath = filepath.Join(sourcePath, "Engine", "Binaries", "Linux", "UnrealEditor")
	}
	if _, err := os.Stat(editorPath); os.IsNotExist(err) {
		s.Status = "fail"
		s.Detail = filepath.Base(editorPath) + " not found"
		return s
	}
	s.Status = "ok"
	s.Detail = filepath.Base(editorPath) + " found"
	return s
}

func checkServerBuild(projectName, outputDir string) stageStatus {
	s := stageStatus{Name: projectName + " Server Build"}
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

func checkClientBuild(projectName string) stageStatus {
	s := stageStatus{Name: projectName + " Client Build"}

	st, err := state.Load()
	if err != nil {
		s.Status = "unknown"
		s.Detail = "could not read state"
		return s
	}

	if st.Client == nil || st.Client.BinaryPath == "" {
		s.Status = "fail"
		s.Detail = "not built"
		return s
	}

	if _, err := os.Stat(st.Client.BinaryPath); os.IsNotExist(err) {
		s.Status = "fail"
		s.Detail = "binary missing: " + st.Client.BinaryPath
		return s
	}

	s.Status = "ok"
	s.Detail = st.Client.OutputDir
	return s
}

func checkGameSession() stageStatus {
	s := stageStatus{Name: "Game Session"}

	st, err := state.Load()
	if err != nil {
		s.Status = "unknown"
		s.Detail = "could not read state"
		return s
	}

	if st.Session == nil {
		s.Status = "fail"
		s.Detail = "no session"
		return s
	}

	s.Status = "ok"
	s.Detail = fmt.Sprintf("%s (%s:%d)", st.Session.SessionID, st.Session.IPAddress, st.Session.Port)
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
