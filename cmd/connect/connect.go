package connect

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/state"
	"github.com/spf13/cobra"
)

var address string

// Cmd is the connect command.
var Cmd = &cobra.Command{
	Use:   "connect",
	Short: "Launch the game client and connect to the active game session",
	Long: `Reads the active game session from .ludus/state.json, verifies
it is still alive via the GameLift API, then launches the game client
binary to connect.

Use --address to override the IP:port instead of reading from state.`,
	RunE: runConnect,
}

func init() {
	Cmd.Flags().StringVar(&address, "address", "", "override server address (ip:port)")
}

func runConnect(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	// Resolve connection address
	ip, port, err := resolveAddress(cmd)
	if err != nil {
		return err
	}

	// Resolve client binary
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if s.Client == nil || s.Client.BinaryPath == "" {
		return fmt.Errorf("no client build found — run 'ludus game client' first")
	}

	binaryPath := s.Client.BinaryPath
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("client binary not found at %s — run 'ludus game client' first", binaryPath)
	}

	// Verify game session is still alive (unless using manual address)
	if address == "" && s.Session != nil {
		awsCfg, err := gamelift.LoadAWSConfig(cmd.Context(), cfg.AWS.Region)
		if err != nil {
			fmt.Printf("Warning: could not verify game session: %v\n", err)
		} else {
			deployer := gamelift.NewDeployer(gamelift.DeployOptions{
				Region: cfg.AWS.Region,
			}, awsCfg)

			status, err := deployer.DescribeGameSession(cmd.Context(), s.Session.SessionID)
			if err != nil {
				return fmt.Errorf("game session check failed: %w", err)
			}

			if status != "ACTIVE" {
				return fmt.Errorf("game session %s is %s — run 'ludus deploy session' to create a new one",
					s.Session.SessionID, status)
			}
		}
	}

	connectAddr := fmt.Sprintf("%s:%d", ip, port)

	projectName := cfg.Game.ProjectName
	clientTarget := cfg.Game.ResolvedClientTarget()
	return launchClient(binaryPath, s.Client.Platform, s.Client.OutputDir, connectAddr, projectName, clientTarget)
}

func resolveAddress(cmd *cobra.Command) (string, int, error) {
	if address != "" {
		parts := strings.SplitN(address, ":", 2)
		if len(parts) != 2 {
			return "", 0, fmt.Errorf("invalid address format: expected ip:port, got %q", address)
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", 0, fmt.Errorf("invalid port in address %q: %w", address, err)
		}
		return parts[0], port, nil
	}

	s, err := state.Load()
	if err != nil {
		return "", 0, fmt.Errorf("loading state: %w", err)
	}

	if s.Session == nil {
		return "", 0, fmt.Errorf("no active game session — run 'ludus deploy session' first")
	}

	return s.Session.IPAddress, s.Session.Port, nil
}
