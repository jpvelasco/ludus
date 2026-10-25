package configcmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/spf13/cobra"
)

// Cmd is the top-level config command group.
var Cmd = &cobra.Command{
	Use:   "config",
	Short: "View and modify ludus.yaml configuration",
}

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value in ludus.yaml",
	Long: `Sets a configuration value in ludus.yaml using dot-notation keys.

Examples:
  ludus config set engine.sourcePath /path/to/UnrealEngine
  ludus config set engine.version 5.8.0
  ludus config set gamelift.instanceType c6g.large
  ludus config set gamelift.fleetName ludus-fleet-ue57
  ludus config set deploy.target ec2
  ludus config set game.serverMap L_Expanse
  ludus config set game.arch arm64
  ludus config set engine.maxJobs 4

Creates ludus.yaml if it does not exist.`,
	Args: cobra.ExactArgs(2),
	RunE: runSet,
}

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value from ludus.yaml",
	Long: `Reads a configuration value from ludus.yaml using dot-notation keys.

Examples:
  ludus config get engine.sourcePath
  ludus config get gamelift.instanceType
  ludus config get deploy.target`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	Cmd.AddCommand(setCmd)
	Cmd.AddCommand(getCmd)
}

func runSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]
	cfgFile := globals.ResolveConfigFile()

	// The document is seeded with the file's current contents so the write
	// preserves every other field.
	doc, _, err := globals.NewConfigDocument(cfgFile)
	if err != nil {
		return err
	}

	// Convert numeric and boolean strings to their typed values
	doc.Set(key, parseValue(value))

	if err := globals.SaveConfigDocument(doc, cfgFile); err != nil {
		return err
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}

func runGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	cfgFile := globals.ResolveConfigFile()

	doc, exists, err := globals.NewConfigDocument(cfgFile)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s not found; run 'ludus config set' or 'ludus setup' to create one", cfgFile)
	}

	if !doc.IsSet(key) {
		return fmt.Errorf("key %q not found in %s", key, cfgFile)
	}

	val := doc.Get(key)
	fmt.Println(formatValue(val))
	return nil
}

// parseValue converts string input to int, float, or bool where appropriate.
func parseValue(s string) any {
	if strings.EqualFold(s, "true") {
		return true
	}
	if strings.EqualFold(s, "false") {
		return false
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		// Only use float if it has a decimal point (avoid converting "0" to 0.0)
		if strings.Contains(s, ".") {
			return f
		}
	}
	return s
}

// formatValue converts a config value to a display string.
func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}
