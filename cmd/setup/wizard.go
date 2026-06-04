package setup

import (
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

// setupAnswers holds all user responses collected during the wizard.
type setupAnswers struct {
	cfgFile           string
	enginePath        string
	engineVersion     string
	projectName       string
	projectPath       string
	contentSourcePath string
	deployTarget      string
	region            string
	accountID         string
	instanceType      string
}

func runSetup(cmd *cobra.Command, args []string) error {
	fmt.Println("Ludus Setup Wizard")
	fmt.Println("==================")
	fmt.Println()

	cfgFile := resolveConfigFile()

	// Load existing config (if any) to use as prompt defaults.
	var existing *config.Config
	if _, err := os.Stat(cfgFile); err == nil {
		if !confirm(fmt.Sprintf("%s already exists. Overwrite?", cfgFile)) {
			fmt.Println("Setup cancelled.")
			return nil
		}
		existing, _ = config.Load(cfgFile)
	}

	a := collectAnswers(cfgFile, existing)

	printSummary(a)

	if !confirm("Write configuration?") {
		fmt.Println("Setup cancelled.")
		return nil
	}

	return writeConfig(a, existing)
}

// resolveConfigFile returns the config file name based on the active profile.
func resolveConfigFile() string {
	if globals.Profile != "" {
		return "ludus-" + globals.Profile + ".yaml"
	}
	return "ludus.yaml"
}

// collectAnswers runs each wizard step and returns the collected responses.
// existing may be nil (first run); when set, its values are used as prompt defaults.
func collectAnswers(cfgFile string, existing *config.Config) setupAnswers {
	a := setupAnswers{cfgFile: cfgFile}

	collectEngineAnswers(&a, existing)
	collectProjectAnswers(&a, existing)
	collectDeploymentAnswers(&a, existing)
	collectAWSAnswers(&a, existing)

	return a
}

func collectEngineAnswers(a *setupAnswers, existing *config.Config) {
	fmt.Println("Step 1: Unreal Engine Source")
	fmt.Println("----------------------------")

	a.enginePath = promptEnginePathDefault(existingString("", existing, func(c *config.Config) string { return c.Engine.SourcePath }))
	if a.enginePath == "" {
		fmt.Println("\nNo engine source path provided. You can set it later:")
		fmt.Printf("  ludus config set engine.sourcePath /path/to/UnrealEngine\n\n")
	}

	a.engineVersion = detectEngineVersion(a.enginePath)
}

func collectProjectAnswers(a *setupAnswers, existing *config.Config) {
	fmt.Println()
	fmt.Println("Step 2: Game Project")
	fmt.Println("--------------------")

	defaultName := existingString("Lyra", existing, func(c *config.Config) string { return c.Game.ProjectName })
	a.projectName, a.projectPath, a.contentSourcePath = promptGameProjectDefault(a.enginePath, defaultName, existing)
}

func collectDeploymentAnswers(a *setupAnswers, existing *config.Config) {
	fmt.Println()
	fmt.Println("Step 3: Deployment Target")
	fmt.Println("-------------------------")
	targets := []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
	a.deployTarget = promptChoice("Select deployment target:", targets, deployTargetDefault(existing, targets))
}

// deployTargetDefault returns the index of the existing deploy target in targets,
// or 0 if not found.
func deployTargetDefault(existing *config.Config, targets []string) int {
	if existing == nil || existing.Deploy.Target == "" {
		return 0
	}
	for i, t := range targets {
		if t == existing.Deploy.Target {
			return i
		}
	}
	return 0
}

func collectAWSAnswers(a *setupAnswers, existing *config.Config) {
	fmt.Println()
	fmt.Println("Step 4: AWS Configuration")
	fmt.Println("-------------------------")

	a.region, a.accountID = promptAWSDefault(existingString("us-east-1", existing, func(c *config.Config) string { return c.AWS.Region }), existing)

	a.instanceType = existingString("c6i.large", existing, func(c *config.Config) string { return c.GameLift.InstanceType })
	if a.deployTarget != "binary" && a.deployTarget != "anywhere" {
		fmt.Println()
		a.instanceType = prompt("GameLift instance type", a.instanceType)
	}
}

// existingString returns the value from existing via getter if existing is non-nil
// and the value is non-empty, otherwise returns defaultVal.
func existingString(defaultVal string, existing *config.Config, getter func(*config.Config) string) string {
	if existing != nil {
		if v := getter(existing); v != "" {
			return v
		}
	}
	return defaultVal
}

// detectEngineVersion auto-detects or prompts for the engine version.
func detectEngineVersion(enginePath string) string {
	if enginePath == "" {
		return ""
	}
	bv, err := toolchain.ParseBuildVersion(enginePath)
	if err == nil {
		version := fmt.Sprintf("%d.%d.%d", bv.MajorVersion, bv.MinorVersion, bv.PatchVersion)
		fmt.Printf("\nDetected engine version: %s\n", version)
		return version
	}
	fmt.Println("\nCould not auto-detect engine version from Build.version.")
	return prompt("Engine version (e.g., 5.7.3)", "")
}

// printSummary displays the collected configuration before writing.
func printSummary(a setupAnswers) {
	fmt.Println()
	fmt.Println("Configuration Summary")
	fmt.Println("=====================")
	printIfSet("  Engine source:  %s\n", a.enginePath)
	printIfSet("  Engine version: %s\n", a.engineVersion)
	fmt.Printf("  Project:        %s\n", a.projectName)
	printIfSet("  Project path:   %s\n", a.projectPath)
	printIfSet("  Content source: %s\n", a.contentSourcePath)
	fmt.Printf("  Deploy target:  %s\n", a.deployTarget)
	printIfSet("  AWS region:     %s\n", a.region)
	printIfSet("  AWS account:    %s\n", a.accountID)
	if a.deployTarget != "binary" && a.deployTarget != "anywhere" {
		fmt.Printf("  Instance type:  %s\n", a.instanceType)
	}
	fmt.Printf("  Config file:    %s\n", a.cfgFile)
	fmt.Println()
}

// printIfSet prints format with value only when value is non-empty.
func printIfSet(format, value string) {
	if value != "" {
		fmt.Printf(format, value)
	}
}
