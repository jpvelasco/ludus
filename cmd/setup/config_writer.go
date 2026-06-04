package setup

import (
	"fmt"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/spf13/viper"
)

// writeConfig writes the ludus.yaml configuration file.
// When existing is non-nil, its values are merged in first so that fields
// not surfaced by the wizard (e.g. engine.backend, ddc.mode) are preserved.
func writeConfig(a setupAnswers, existing *config.Config) error {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(a.cfgFile)

	// Preserve un-prompted fields from the existing config.
	if existing != nil {
		if existing.Engine.Backend != "" {
			v.Set("engine.backend", existing.Engine.Backend)
		}
		if existing.Engine.DockerImage != "" {
			v.Set("engine.dockerImage", existing.Engine.DockerImage)
		}
		if existing.Engine.DockerImageName != "" {
			v.Set("engine.dockerImageName", existing.Engine.DockerImageName)
		}
		if existing.Engine.DockerBaseImage != "" {
			v.Set("engine.dockerBaseImage", existing.Engine.DockerBaseImage)
		}
		if existing.Engine.MaxJobs != 0 {
			v.Set("engine.maxJobs", existing.Engine.MaxJobs)
		}
		if existing.DDC.Mode != "" {
			v.Set("ddc.mode", existing.DDC.Mode)
		}
		if existing.DDC.LocalPath != "" {
			v.Set("ddc.localPath", existing.DDC.LocalPath)
		}
		if existing.Game.Arch != "" {
			v.Set("game.arch", existing.Game.Arch)
		}
		if existing.Game.Platform != "" {
			v.Set("game.platform", existing.Game.Platform)
		}
		if existing.Game.ServerTarget != "" {
			v.Set("game.serverTarget", existing.Game.ServerTarget)
		}
		if existing.Game.ServerMap != "" {
			v.Set("game.serverMap", existing.Game.ServerMap)
		}
		if existing.AWS.ECRRepository != "" {
			v.Set("aws.ecrRepository", existing.AWS.ECRRepository)
		}
		if len(existing.AWS.Tags) > 0 {
			v.Set("aws.tags", existing.AWS.Tags)
		}
	}

	setEngineConfig(v, a)
	setGameConfig(v, a)
	setDeployConfig(v, a)
	setContainerConfig(v)

	if err := v.WriteConfigAs(a.cfgFile); err != nil {
		return fmt.Errorf("writing %s: %w", a.cfgFile, err)
	}

	fmt.Printf("\nConfiguration written to %s\n", a.cfgFile)
	fmt.Println("\nNext: ludus init")
	return nil
}

// setEngineConfig writes engine settings to Viper.
func setEngineConfig(v *viper.Viper, a setupAnswers) {
	if a.enginePath != "" {
		v.Set("engine.sourcePath", a.enginePath)
	}
	if a.engineVersion != "" {
		v.Set("engine.version", a.engineVersion)
	}
}

// setGameConfig writes game project settings to Viper.
func setGameConfig(v *viper.Viper, a setupAnswers) {
	v.Set("game.projectName", a.projectName)
	if a.projectPath != "" {
		v.Set("game.projectPath", a.projectPath)
	}
	if a.contentSourcePath != "" {
		v.Set("game.contentSourcePath", a.contentSourcePath)
	}
	v.Set("game.serverMap", "L_Expanse")
}

// setDeployConfig writes AWS and deployment settings to Viper.
func setDeployConfig(v *viper.Viper, a setupAnswers) {
	v.Set("deploy.target", a.deployTarget)

	if a.region != "" {
		v.Set("aws.region", a.region)
	} else {
		v.Set("aws.region", "us-east-1")
	}
	if a.accountID != "" {
		v.Set("aws.accountId", a.accountID)
	}
	v.Set("aws.ecrRepository", "ludus-server")

	v.Set("gamelift.fleetName", "ludus-fleet")
	v.Set("gamelift.instanceType", a.instanceType)
	v.Set("gamelift.containerGroupName", "ludus-container-group")
}

// setContainerConfig writes container settings to Viper.
func setContainerConfig(v *viper.Viper) {
	v.Set("container.imageName", "ludus-server")
	v.Set("container.tag", "latest")
	v.Set("container.serverPort", 7777)
}
