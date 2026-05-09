package setup

import (
	"fmt"

	"github.com/spf13/viper"
)

// writeConfig creates and writes the ludus.yaml configuration file.
func writeConfig(a setupAnswers) error {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(a.cfgFile)

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
