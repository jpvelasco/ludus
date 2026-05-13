package setup

import (
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/cmd/globals"
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

	if _, err := os.Stat(cfgFile); err == nil {
		if !confirm(fmt.Sprintf("%s already exists. Overwrite?", cfgFile)) {
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	a := collectAnswers(cfgFile)

	printSummary(a)

	if !confirm("Write configuration?") {
		fmt.Println("Setup cancelled.")
		return nil
	}

	return writeConfig(a)
}

// resolveConfigFile returns the config file name based on the active profile.
func resolveConfigFile() string {
	if globals.Profile != "" {
		return "ludus-" + globals.Profile + ".yaml"
	}
	return "ludus.yaml"
}

// collectAnswers runs each wizard step and returns the collected responses.
func collectAnswers(cfgFile string) setupAnswers {
	a := setupAnswers{cfgFile: cfgFile}

	collectEngineAnswers(&a)
	collectProjectAnswers(&a)
	collectDeploymentAnswers(&a)
	collectAWSAnswers(&a)

	return a
}

func collectEngineAnswers(a *setupAnswers) {
	fmt.Println("Step 1: Unreal Engine Source")
	fmt.Println("----------------------------")
	a.enginePath = promptEnginePath()
	if a.enginePath == "" {
		fmt.Println("\nNo engine source path provided. You can set it later:")
		fmt.Printf("  ludus config set engine.sourcePath /path/to/UnrealEngine\n\n")
	}

	a.engineVersion = detectEngineVersion(a.enginePath)
}

func collectProjectAnswers(a *setupAnswers) {
	fmt.Println()
	fmt.Println("Step 2: Game Project")
	fmt.Println("--------------------")
	a.projectName, a.projectPath, a.contentSourcePath = promptGameProject(a.enginePath)
}

func collectDeploymentAnswers(a *setupAnswers) {
	fmt.Println()
	fmt.Println("Step 3: Deployment Target")
	fmt.Println("-------------------------")
	targets := []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
	a.deployTarget = promptChoice("Select deployment target:", targets, 0)
}

func collectAWSAnswers(a *setupAnswers) {
	fmt.Println()
	fmt.Println("Step 4: AWS Configuration")
	fmt.Println("-------------------------")
	a.region, a.accountID = promptAWS()

	a.instanceType = "c6i.large"
	if a.deployTarget != "binary" && a.deployTarget != "anywhere" {
		fmt.Println()
		a.instanceType = prompt("GameLift instance type", "c6i.large")
	}
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
