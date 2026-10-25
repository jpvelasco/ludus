package setup

import (
	"fmt"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/spf13/viper"
)

// writeConfig writes the wizard answers to the config file. The document is
// seeded with the file's current contents so every setting the wizard does
// not prompt for (ddc.zenPath, privacy, observability, deploy and EC2
// settings, custom ECR repository, ...) survives a re-run; only the fields
// the wizard edits are overwritten (#703).
func writeConfig(a setupAnswers) error {
	// Read errors are tolerated: runSetup confirms overwriting an unreadable
	// file with the user first, and NewConfigDocument returns a blank
	// document alongside the error to write from scratch.
	v, _, _ := globals.NewConfigDocument(a.cfgFile)

	setEngineConfig(v, a)
	setGameConfig(v, a)
	setDeployConfig(v, a)
	setContainerConfig(v)

	if err := globals.SaveConfigDocument(v, a.cfgFile); err != nil {
		return err
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

// setGameConfig writes game project settings to Viper. Fields the wizard
// does not prompt for (serverMap) are only defaulted when the document does
// not carry them, so a re-run preserves values set outside the wizard.
func setGameConfig(v *viper.Viper, a setupAnswers) {
	v.Set("game.projectName", a.projectName)
	if a.projectPath != "" {
		v.Set("game.projectPath", a.projectPath)
	}
	if a.contentSourcePath != "" {
		v.Set("game.contentSourcePath", a.contentSourcePath)
	}
	setDefault(v, "game.serverMap", "L_Expanse")
}

// setDeployConfig writes AWS and deployment settings to Viper. Keys the
// wizard never prompts for keep their seeded values; the defaults below only
// apply to a fresh document.
func setDeployConfig(v *viper.Viper, a setupAnswers) {
	v.Set("deploy.target", a.deployTarget)

	if a.region != "" {
		v.Set("aws.region", a.region)
	} else {
		setDefault(v, "aws.region", "us-east-1")
	}
	if a.accountID != "" {
		v.Set("aws.accountId", a.accountID)
	}
	v.Set("gamelift.instanceType", a.instanceType)

	setDefault(v, "aws.ecrRepository", "ludus-server")
	setDefault(v, "gamelift.fleetName", "ludus-fleet")
	setDefault(v, "gamelift.containerGroupName", "ludus-container-group")
}

// setContainerConfig writes container settings to Viper, defaulting only
// keys the document does not carry yet.
func setContainerConfig(v *viper.Viper) {
	setDefault(v, "container.imageName", "ludus-server")
	setDefault(v, "container.tag", "latest")
	setDefault(v, "container.serverPort", 7777)
}

// setDefault sets key to val only when the document has no value for it.
func setDefault(v *viper.Viper, key string, val any) {
	if !v.IsSet(key) {
		v.Set(key, val)
	}
}
