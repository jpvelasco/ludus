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
  - Deploy:    Is the target deployed? Active? Game sessions?`,
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

	// 6. Deploy target
	stages = append(stages, checkDeployTarget(cmd, cfg))

	// 7. Game session (only relevant if target supports sessions)
	stages = append(stages, checkGameSession(cfg))

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

func checkDeployTarget(cmd *cobra.Command, cfg *config.Config) stageStatus {
	targetName := cfg.Deploy.Target
	if targetName == "" {
		targetName = "gamelift"
	}

	s := stageStatus{Name: strings.ToUpper(targetName[:1]) + targetName[1:] + " Deployment"}

	target, err := globals.ResolveTarget(cmd.Context(), cfg, "")
	if err != nil {
		s.Status = "unknown"
		s.Detail = fmt.Sprintf("could not resolve target: %v", err)
		return s
	}

	ds, err := target.Status(cmd.Context())
	if err != nil {
		s.Status = "unknown"
		s.Detail = fmt.Sprintf("status check error: %v", err)
		return s
	}

	switch ds.Status {
	case "active":
		s.Status = "ok"
	case "not_deployed":
		s.Status = "fail"
	default:
		s.Status = "unknown"
	}
	s.Detail = ds.Detail
	return s
}

func checkGameSession(cfg *config.Config) stageStatus {
	s := stageStatus{Name: "Game Session"}

	// Only show session status if the target supports sessions
	targetName := cfg.Deploy.Target
	if targetName == "" {
		targetName = "gamelift"
	}
	if targetName != "gamelift" {
		s.Status = "unknown"
		s.Detail = fmt.Sprintf("not applicable for %s target", targetName)
		return s
	}

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
