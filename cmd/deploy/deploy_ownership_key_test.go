package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestConfiguredOwnershipKeyReachesFleetPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ludus.yaml")
	if err := os.WriteFile(path, []byte("gamelift:\n  ownershipTagKey: Deployment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testGameliftCfg()
	cfg.GameLift = loaded.GameLift
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	saveDeployFlags(t)
	fleetName = "flag-fleet"
	var runErr error
	out := captureStdout(func() { runErr = runFleet(newCommand(), nil) })
	if runErr != nil || !strings.Contains(out, "Deployment:flag-fleet") {
		t.Fatalf("configured ownership missing: %v\n%s", runErr, out)
	}
	if strings.Contains(out, "ludus:fleet-name:") {
		t.Fatalf("legacy key still written: %s", out)
	}
}

func TestDeployerRejectsReservedOwnershipKeyBeforeAWS(t *testing.T) {
	cfg := testGameliftCfg()
	cfg.GameLift.OwnershipTagKey = "Project"
	globals.SetGlobals(t, cfg)
	saveDeployFlags(t)
	if _, err := makeDeployer(newCommand()); err == nil || !strings.Contains(err.Error(), "cannot replace") {
		t.Fatalf("reserved ownership key accepted: %v", err)
	}
}
