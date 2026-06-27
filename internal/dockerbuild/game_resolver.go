package dockerbuild

import (
	"fmt"
	"path/filepath"

	"github.com/jpvelasco/ludus/internal/config"
)

// resolveProjectName returns the project name, defaulting to "Lyra".
func (b *DockerGameBuilder) resolveProjectName() string {
	if b.opts.ProjectName != "" {
		return b.opts.ProjectName
	}
	return "Lyra"
}

// resolveServerTarget returns the server target, defaulting to ProjectName + "Server".
func (b *DockerGameBuilder) resolveServerTarget() string {
	if b.opts.ServerTarget != "" {
		return b.opts.ServerTarget
	}
	return b.resolveProjectName() + "Server"
}

// resolveGameTarget returns the game target, defaulting to ProjectName + "Game".
func (b *DockerGameBuilder) resolveGameTarget() string {
	if b.opts.GameTarget != "" {
		return b.opts.GameTarget
	}
	return b.resolveProjectName() + "Game"
}

// resolveArch returns normalized arch (default amd64).
func (b *DockerGameBuilder) resolveArch() string {
	if b.opts.Arch != "" {
		return config.NormalizeArch(b.opts.Arch)
	}
	return "amd64"
}

// resolveClientPlatform returns "Linux" or "LinuxArm64" based on arch.
func (b *DockerGameBuilder) resolveClientPlatform() string {
	if b.opts.ClientPlatform != "" {
		return b.opts.ClientPlatform
	}
	if b.resolveArch() == "arm64" {
		return "LinuxArm64"
	}
	return "Linux"
}

// isExternalProject returns true if the project is outside the engine tree
// and needs to be volume-mounted into the container.
func (b *DockerGameBuilder) isExternalProject() bool {
	return b.opts.ProjectPath != ""
}

// containerProjectPath returns the project path as seen from inside the container.
func (b *DockerGameBuilder) containerProjectPath() string {
	if b.isExternalProject() {
		// The project directory is mounted at /project, so the .uproject lives at
		// /project/<basename>. Derive the filename from the path the user gave —
		// it can legitimately differ from projectName, which only governs target
		// names (e.g. Epic ships LyraStarterGame.uproject with LyraServer targets).
		return "/project/" + filepath.Base(b.opts.ProjectPath)
	}
	// Lyra or in-engine project
	return fmt.Sprintf("/engine/Samples/Games/%s/%s.uproject",
		b.resolveProjectName(), b.resolveProjectName())
}
