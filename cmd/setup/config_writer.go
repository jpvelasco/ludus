package setup

import (
	"fmt"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/spf13/viper"
)

// writeConfig writes the ludus.yaml configuration file.
// When existing is non-nil, its values are seeded first so that fields
// not surfaced by the wizard (e.g. engine.backend, ddc.mode) are preserved.
func writeConfig(a setupAnswers, existing *config.Config) error {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(a.cfgFile)

	preserveExistingFields(v, existing)

	setEngineConfig(v, a)
	setGameConfig(v, a, existing)
	setDeployConfig(v, a)
	setContainerConfig(v)

	if err := v.WriteConfigAs(a.cfgFile); err != nil {
		return fmt.Errorf("writing %s: %w", a.cfgFile, err)
	}

	fmt.Printf("\nConfiguration written to %s\n", a.cfgFile)
	fmt.Println("\nNext: ludus init")
	return nil
}

// preserveExistingFields seeds v with fields from existing that the wizard
// does not prompt for, so they survive a setup re-run.
func preserveExistingFields(v *viper.Viper, existing *config.Config) {
	if existing == nil {
		return
	}
	setIfNonEmpty := func(key, val string) {
		if val != "" {
			v.Set(key, val)
		}
	}
	setIfNonEmpty("engine.backend", existing.Engine.Backend)
	setIfNonEmpty("engine.dockerImage", existing.Engine.DockerImage)
	setIfNonEmpty("engine.dockerImageName", existing.Engine.DockerImageName)
	setIfNonEmpty("engine.dockerBaseImage", existing.Engine.DockerBaseImage)
	setIfNonEmpty("ddc.mode", existing.DDC.Mode)
	setIfNonEmpty("ddc.localPath", existing.DDC.LocalPath)
	setIfNonEmpty("game.arch", existing.Game.Arch)
	setIfNonEmpty("game.platform", existing.Game.Platform)
	setIfNonEmpty("game.serverTarget", existing.Game.ServerTarget)
	setIfNonEmpty("aws.ecrRepository", existing.AWS.ECRRepository)
	if existing.Engine.MaxJobs != 0 {
		v.Set("engine.maxJobs", existing.Engine.MaxJobs)
	}
	if len(existing.AWS.Tags) > 0 {
		v.Set("aws.tags", existing.AWS.Tags)
	}
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
// existing is used to preserve serverMap when it was set outside the wizard.
func setGameConfig(v *viper.Viper, a setupAnswers, existing *config.Config) {
	v.Set("game.projectName", a.projectName)
	if a.projectPath != "" {
		v.Set("game.projectPath", a.projectPath)
	}
	if a.contentSourcePath != "" {
		v.Set("game.contentSourcePath", a.contentSourcePath)
	}
	// Preserve existing serverMap; only fall back to the default on first run.
	if existing != nil && existing.Game.ServerMap != "" {
		v.Set("game.serverMap", existing.Game.ServerMap)
	} else {
		v.Set("game.serverMap", "L_Expanse")
	}
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
