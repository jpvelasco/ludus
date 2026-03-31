package connect

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/prereq"
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
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckAWSReady()); err != nil {
		return err
	}

	ip, port, err := resolveAddress()
	if err != nil {
		return err
	}

	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	binaryPath, err := resolveClientBinary(s)
	if err != nil {
		return err
	}

	if err := verifySession(cmd, s); err != nil {
		return err
	}

	connectAddr := fmt.Sprintf("%s:%d", ip, port)
	return launchClient(binaryPath, s.Client.Platform, s.Client.OutputDir, connectAddr, globals.Cfg.Game.ResolvedClientTarget())
}

// resolveClientBinary validates that a client build exists and returns its path.
func resolveClientBinary(s *state.State) (string, error) {
	if s.Client == nil || s.Client.BinaryPath == "" {
		return "", fmt.Errorf("no client build found — run 'ludus game client' first")
	}
	if _, err := os.Stat(s.Client.BinaryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("client binary not found at %s — run 'ludus game client' first", s.Client.BinaryPath)
	}
	return s.Client.BinaryPath, nil
}

// verifySession checks that an active game session exists and is still alive.
// When --address is provided, session verification is skipped entirely.
func verifySession(cmd *cobra.Command, s *state.State) error {
	if address != "" {
		return nil
	}
	if s.Session != nil {
		return verifyActiveSession(cmd, s)
	}
	return checkSessionlessTarget(cmd)
}

// verifyActiveSession confirms the stored session is still ACTIVE.
func verifyActiveSession(cmd *cobra.Command, s *state.State) error {
	target, err := globals.ResolveTarget(cmd.Context(), globals.Cfg, "")
	if err != nil {
		fmt.Printf("Warning: could not resolve deploy target: %v\n", err)
		return nil
	}
	sm, ok := target.(deploy.SessionManager)
	if !ok {
		return nil
	}
	status, err := sm.DescribeSession(cmd.Context(), s.Session.SessionID)
	if err != nil {
		return fmt.Errorf("game session check failed: %w", err)
	}
	if status != "ACTIVE" {
		return fmt.Errorf("game session %s is %s — run 'ludus deploy session' to create a new one",
			s.Session.SessionID, status)
	}
	return nil
}

// checkSessionlessTarget returns an error if the target does not support sessions.
func checkSessionlessTarget(cmd *cobra.Command) error {
	target, err := globals.ResolveTarget(cmd.Context(), globals.Cfg, "")
	if err != nil {
		return nil
	}
	if !target.Capabilities().SupportsSession {
		return fmt.Errorf("target %q does not support game sessions — use --address to connect directly", target.Name())
	}
	return nil
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
