package setup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Cmd is the setup wizard command.
var Cmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard for first-time configuration",
	Long: `Guided setup that scans your system, auto-detects settings, and writes
a complete ludus.yaml configuration file.

Steps:
  1. Locate Unreal Engine source directory
  2. Auto-detect engine version from Build.version
  3. Configure game project (Lyra or custom)
  4. Choose deployment target
  5. Configure AWS settings (optional)
  6. Write ludus.yaml

Use --profile to create a profile-specific config (ludus-<profile>.yaml).`,
	RunE: runSetup,
}

var scanner *bufio.Scanner

func init() {
	scanner = bufio.NewScanner(os.Stdin)
}

// prompt displays a question with an optional default and reads user input.
func prompt(question, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return defaultVal
	}
	return answer
}

// promptChoice displays a question with numbered choices and returns the selected value.
func promptChoice(question string, choices []string, defaultIdx int) string {
	fmt.Println(question)
	for i, c := range choices {
		marker := "  "
		if i == defaultIdx {
			marker = "* "
		}
		fmt.Printf("  %s%d) %s\n", marker, i+1, c)
	}
	fmt.Printf("Choice [%d]: ", defaultIdx+1)
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return choices[defaultIdx]
	}
	// Parse number
	for i, c := range choices {
		if answer == fmt.Sprintf("%d", i+1) || strings.EqualFold(answer, c) {
			return c
		}
	}
	return choices[defaultIdx]
}

// confirm asks a yes/no question.
func confirm(question string) bool {
	fmt.Printf("%s [Y/n]: ", question)
	scanner.Scan()
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}

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

	fmt.Println("Step 1: Unreal Engine Source")
	fmt.Println("----------------------------")
	a.enginePath = promptEnginePath()
	if a.enginePath == "" {
		fmt.Println("\nNo engine source path provided. You can set it later:")
		fmt.Printf("  ludus config set engine.sourcePath /path/to/UnrealEngine\n\n")
	}

	a.engineVersion = detectEngineVersion(a.enginePath)

	fmt.Println()
	fmt.Println("Step 2: Game Project")
	fmt.Println("--------------------")
	a.projectName, a.projectPath, a.contentSourcePath = promptGameProject(a.enginePath)

	fmt.Println()
	fmt.Println("Step 3: Deployment Target")
	fmt.Println("-------------------------")
	targets := []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
	a.deployTarget = promptChoice("Select deployment target:", targets, 0)

	fmt.Println()
	fmt.Println("Step 4: AWS Configuration")
	fmt.Println("-------------------------")
	a.region, a.accountID = promptAWS()

	a.instanceType = "c6i.large"
	if a.deployTarget != "binary" && a.deployTarget != "anywhere" {
		fmt.Println()
		a.instanceType = prompt("GameLift instance type", "c6i.large")
	}

	return a
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

// promptEnginePath scans for engine directories and lets the user pick or type a path.
func promptEnginePath() string {
	candidates := scanEnginePaths()

	if len(candidates) > 0 {
		fmt.Println("Found engine source directories:")
		for i, c := range candidates {
			version := ""
			if bv, err := toolchain.ParseBuildVersion(c); err == nil {
				version = fmt.Sprintf(" (v%d.%d.%d)", bv.MajorVersion, bv.MinorVersion, bv.PatchVersion)
			}
			fmt.Printf("  %d) %s%s\n", i+1, c, version)
		}
		fmt.Printf("  %d) Enter a different path\n", len(candidates)+1)
		fmt.Printf("  %d) Skip (configure later)\n", len(candidates)+2)
		fmt.Printf("Choice [1]: ")
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer == "" {
			answer = "1"
		}
		for i, c := range candidates {
			if answer == fmt.Sprintf("%d", i+1) {
				return c
			}
		}
		if answer == fmt.Sprintf("%d", len(candidates)+1) {
			return prompt("Engine source path", "")
		}
		// Skip
		return ""
	}

	return prompt("Engine source path (or press Enter to skip)", "")
}

// scanEnginePaths looks for Unreal Engine source directories in common locations.
func scanEnginePaths() []string {
	var candidates []string
	seen := make(map[string]bool)

	addIfEngine := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if seen[abs] {
			return
		}
		// Check for Setup.bat (Windows) or Setup.sh (Linux) as engine marker
		var marker string
		if runtime.GOOS == "windows" {
			marker = filepath.Join(abs, "Setup.bat")
		} else {
			marker = filepath.Join(abs, "Setup.sh")
		}
		if _, err := os.Stat(marker); err == nil {
			candidates = append(candidates, abs)
			seen[abs] = true
		}
	}

	// Check additional working directories from the environment (if passed in)
	// Scan parent directories of CWD for UnrealEngine-* patterns
	cwd, _ := os.Getwd()
	if cwd != "" {
		parent := filepath.Dir(cwd)
		scanGlob(parent, "UnrealEngine*", addIfEngine)
	}

	// Common locations
	home, _ := os.UserHomeDir()
	if home != "" {
		scanGlob(filepath.Join(home, "Documents", "Source"), "UnrealEngine*", addIfEngine)
		scanGlob(filepath.Join(home, "Source"), "UnrealEngine*", addIfEngine)
	}

	if runtime.GOOS == "windows" {
		// Scan drive roots
		for _, drive := range []string{"C:", "D:", "E:", "F:"} {
			scanGlob(filepath.Join(drive, string(os.PathSeparator), "Source Code"), "UnrealEngine*", addIfEngine)
			scanGlob(filepath.Join(drive, string(os.PathSeparator), "Source"), "UnrealEngine*", addIfEngine)
			scanGlob(filepath.Join(drive, string(os.PathSeparator)), "UnrealEngine*", addIfEngine)
		}
	} else {
		scanGlob("/opt", "UnrealEngine*", addIfEngine)
		scanGlob("/usr/local/src", "UnrealEngine*", addIfEngine)
	}

	return candidates
}

// scanGlob searches for directories matching pattern inside dir and calls fn for each.
func scanGlob(dir, pattern string, fn func(string)) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return
	}
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}
		fn(m)
	}
}

// promptGameProject asks about the game project configuration.
func promptGameProject(enginePath string) (projectName, projectPath, contentSourcePath string) {
	projectName = prompt("Project name", "Lyra")

	if projectName == "Lyra" && enginePath != "" {
		// Check if Lyra content is already in place
		lyraDir := filepath.Join(enginePath, "Samples", "Games", "Lyra")
		uproject := filepath.Join(lyraDir, "Lyra.uproject")
		if _, err := os.Stat(uproject); err == nil {
			fmt.Printf("  Found Lyra at %s\n", lyraDir)
		}

		// Try to discover downloaded Lyra content
		contentSourcePath = discoverLyraContent()
		if contentSourcePath != "" {
			fmt.Printf("  Found Lyra content download at %s\n", contentSourcePath)
			if !confirm("  Use this as content source?") {
				contentSourcePath = ""
			}
		}
		if contentSourcePath == "" {
			contentSourcePath = prompt("  Lyra content source path (or press Enter to skip)", "")
		}
	} else {
		// Custom project
		projectPath = prompt("Path to .uproject file", "")
		if projectPath != "" {
			// Validate
			if _, err := os.Stat(projectPath); os.IsNotExist(err) {
				fmt.Printf("  Warning: %s not found\n", projectPath)
			}
		}
	}

	return projectName, projectPath, contentSourcePath
}

// discoverLyraContent scans common paths for downloaded Lyra content.
// Mirrors the logic in internal/prereq/checker.go.
func discoverLyraContent() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	var candidates []string
	candidates = append(candidates,
		filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame"),
		filepath.Join(home, "Documents", "Unreal Projects", "Lyra Starter Game"),
	)

	if runtime.GOOS == "windows" {
		if oneDrive := os.Getenv("OneDrive"); oneDrive != "" {
			candidates = append(candidates,
				filepath.Join(oneDrive, "Documents", "Unreal Projects", "LyraStarterGame"),
				filepath.Join(oneDrive, "Documents", "Unreal Projects", "Lyra Starter Game"),
			)
		}
	}

	for _, c := range candidates {
		if isLyraProject(c) {
			return c
		}
	}

	// Try versioned directories
	docsDir := filepath.Join(home, "Documents", "Unreal Projects")
	matches, _ := filepath.Glob(filepath.Join(docsDir, "LyraStarterGame*"))
	for _, m := range matches {
		if isLyraProject(m) {
			return m
		}
	}

	return ""
}

// isLyraProject checks if a directory looks like a Lyra project download.
func isLyraProject(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "Lyra.uproject")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "Content", "DefaultGameData.uasset")); err == nil {
		return true
	}
	return false
}

// promptAWS asks about AWS configuration.
func promptAWS() (region, accountID string) {
	region = prompt("AWS region", "us-east-1")

	// Try to auto-detect account ID
	accountID = detectAWSAccountID()
	if accountID != "" {
		fmt.Printf("  Detected AWS account: %s\n", accountID)
		if !confirm("  Use this account?") {
			accountID = prompt("  AWS account ID", "")
		}
	} else {
		fmt.Println("  Could not detect AWS account (AWS CLI not configured or not installed).")
		accountID = prompt("  AWS account ID (or press Enter to skip)", "")
	}

	return region, accountID
}

// detectAWSAccountID runs aws sts get-caller-identity to detect the account.
func detectAWSAccountID() string {
	if _, err := exec.LookPath("aws"); err != nil {
		return ""
	}
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var identity struct {
		Account string `json:"Account"`
	}
	if json.Unmarshal(out, &identity) != nil {
		return ""
	}
	return identity.Account
}
