package connect

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/state"
	"github.com/spf13/cobra"
)

var address string

// Cmd is the connect command.
var Cmd = &cobra.Command{
	Use:   "connect",
	Short: "Launch the game client and connect to the active game session",
	Long: `Reads the active game session from .ludus/state.json, verifies
it is still alive via the deployment target API, then launches the game client
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
	ip, port, err := resolveAddress()
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
		target, err := globals.ResolveTarget(cmd.Context(), cfg, "")
		if err != nil {
			fmt.Printf("Warning: could not resolve deploy target: %v\n", err)
		} else if sm, ok := target.(deploy.SessionManager); ok {
			status, err := sm.DescribeSession(cmd.Context(), s.Session.SessionID)
			if err != nil {
				return fmt.Errorf("game session check failed: %w", err)
			}
			if status != "ACTIVE" {
				return fmt.Errorf("game session %s is %s — run 'ludus deploy session' to create a new one",
					s.Session.SessionID, status)
			}
		}
		// If target doesn't support sessions, skip verification
	}

	// If no address flag and no session, but target doesn't support sessions,
	// give a clear error
	if address == "" && s.Session == nil {
		target, resolveErr := globals.ResolveTarget(cmd.Context(), cfg, "")
		if resolveErr == nil && !target.Capabilities().SupportsSession {
			return fmt.Errorf("target %q does not support game sessions — use --address to connect directly", target.Name())
		}
	}

	connectAddr := fmt.Sprintf("%s:%d", ip, port)

	projectName := cfg.Game.ProjectName
	clientTarget := cfg.Game.ResolvedClientTarget()
	return launchClient(binaryPath, s.Client.Platform, s.Client.OutputDir, connectAddr, projectName, clientTarget)
}

func resolveAddress() (string, int, error) {
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
