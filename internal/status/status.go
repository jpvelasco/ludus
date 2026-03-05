package status

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/state"
)

// StageStatus represents the status of a single pipeline stage.
type StageStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// CheckAll runs all status checks and returns stage statuses.
func CheckAll(ctx context.Context, cfg *config.Config, target deploy.Target) []StageStatus {
	var stages []StageStatus

	stages = append(stages, CheckEngineSource(cfg.Engine.SourcePath))
	stages = append(stages, CheckEngineBuild(cfg.Engine.SourcePath))

	gameOutputDir := ""
	if cfg.Game.ProjectPath != "" {
		gameOutputDir = filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer")
	} else if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		gameOutputDir = filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer")
	}
	stages = append(stages, CheckServerBuild(cfg.Game.ProjectName, gameOutputDir, cfg.Game.ResolvedArch()))
	stages = append(stages, CheckContainerImage(cfg.Container.ImageName))
	stages = append(stages, CheckClientBuild(cfg.Game.ProjectName))
	stages = append(stages, CheckDeployTarget(ctx, target, cfg.Deploy.Target))
	stages = append(stages, CheckGameSession(cfg))

	return stages
}

// CheckEngineSource checks whether the engine source directory exists.
func CheckEngineSource(sourcePath string) StageStatus {
	s := StageStatus{Name: "Engine Source"}
	if sourcePath == "" {
		s.Status = "fail"
		s.Detail = "not configured"
		return s
	}
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

// CheckEngineBuild checks whether the engine editor binary exists.
func CheckEngineBuild(sourcePath string) StageStatus {
	s := StageStatus{Name: "Engine Build"}
	if sourcePath == "" {
		s.Status = "unknown"
		s.Detail = "source path not configured"
		return s
	}
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

// CheckServerBuild checks whether the packaged server exists.
// arch should be "amd64" or "arm64" (defaults to "amd64" if empty).
func CheckServerBuild(projectName, outputDir, arch string) StageStatus {
	s := StageStatus{Name: projectName + " Server Build"}
	if outputDir == "" {
		s.Status = "unknown"
		s.Detail = "output directory unknown"
		return s
	}
	serverDir := filepath.Join(outputDir, config.ServerPlatformDir(arch))
	if _, err := os.Stat(serverDir); os.IsNotExist(err) {
		s.Status = "fail"
		s.Detail = "not built"
		return s
	}
	s.Status = "ok"
	s.Detail = serverDir
	return s
}

// CheckContainerImage checks whether the Docker image exists.
func CheckContainerImage(imageName string) StageStatus {
	s := StageStatus{Name: "Container Image"}
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

// CheckClientBuild checks whether the client binary exists.
func CheckClientBuild(projectName string) StageStatus {
	s := StageStatus{Name: projectName + " Client Build"}

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

// CheckDeployTarget checks the deployment target status.
func CheckDeployTarget(ctx context.Context, target deploy.Target, targetName string) StageStatus {
	if targetName == "" {
		targetName = "gamelift"
	}

	s := StageStatus{Name: strings.ToUpper(targetName[:1]) + targetName[1:] + " Deployment"}

	if target == nil {
		s.Status = "unknown"
		s.Detail = "target not resolved"
		return s
	}

	ds, err := target.Status(ctx)
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

// CheckGameSession checks for an active game session.
func CheckGameSession(cfg *config.Config) StageStatus {
	s := StageStatus{Name: "Game Session"}

	targetName := cfg.Deploy.Target
	if targetName == "" {
		targetName = "gamelift"
	}
	if targetName != "gamelift" && targetName != "stack" && targetName != "anywhere" && targetName != "ec2" {
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
