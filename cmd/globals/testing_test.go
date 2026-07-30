package globals

import (
	"context"
	"errors"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

// snapshot captures every global SetGlobals is responsible for, so a test can
// prove the state it saw before and after a nested subtest is identical.
type snapshot struct {
	cfg         *config.Config
	verbose     bool
	dryRun      bool
	jsonOutput  bool
	profile     string
	ddcMode     string
	showAccount bool
	noLogs      bool
	commandName string
}

func takeSnapshot() snapshot {
	return snapshot{
		cfg:         Cfg,
		verbose:     Verbose,
		dryRun:      DryRun,
		jsonOutput:  JSONOutput,
		profile:     Profile,
		ddcMode:     DDCMode,
		showAccount: ShowAccountID,
		noLogs:      NoLogs,
		commandName: CommandName,
	}
}

// TestSetGlobalsAppliesEveryOption asserts each option reaches its global.
// Not parallel: SetGlobals mutates package state.
func TestSetGlobalsAppliesEveryOption(t *testing.T) {
	cfg := &config.Config{}

	SetGlobals(t, cfg,
		WithVerbose(true),
		WithDryRun(true),
		WithJSONOutput(true),
		WithShowAccountID(true),
		WithNoLogs(true),
		WithProfile("staging"),
		WithDDCMode("none"),
		WithCommandName("engine"),
	)

	if Cfg != cfg {
		t.Errorf("Cfg = %p, want the config passed to SetGlobals (%p)", Cfg, cfg)
	}

	bools := map[string]bool{
		"Verbose": Verbose, "DryRun": DryRun, "JSONOutput": JSONOutput,
		"ShowAccountID": ShowAccountID, "NoLogs": NoLogs,
	}
	for name, got := range bools {
		if !got {
			t.Errorf("%s = false, want true", name)
		}
	}

	strs := map[string][2]string{
		"Profile":     {Profile, "staging"},
		"DDCMode":     {DDCMode, "none"},
		"CommandName": {CommandName, "engine"},
	}
	for name, pair := range strs {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
}

// TestSetGlobalsRestoresOnCleanup asserts every global returns to its prior
// value after the subtest that set it finishes.
func TestSetGlobalsRestoresOnCleanup(t *testing.T) {
	before := takeSnapshot()

	t.Run("mutates", func(t *testing.T) {
		SetGlobals(t, &config.Config{Deploy: config.DeployConfig{Target: "binary"}},
			WithVerbose(!before.verbose),
			WithDryRun(!before.dryRun),
			WithJSONOutput(!before.jsonOutput),
			WithShowAccountID(!before.showAccount),
			WithNoLogs(!before.noLogs),
			WithProfile(before.profile+"-changed"),
			WithDDCMode(before.ddcMode+"-changed"),
			WithCommandName(before.commandName+"-changed"),
		)
		if Cfg.Deploy.Target != "binary" {
			t.Fatalf("Cfg.Deploy.Target = %q, want %q inside the subtest", Cfg.Deploy.Target, "binary")
		}
	})

	if got := takeSnapshot(); got != before {
		t.Errorf("globals after cleanup = %+v, want %+v", got, before)
	}
}

// TestSetGlobalsLeavesUnsetGlobalsAlone asserts an option that is not passed
// does not clobber the existing value with a zero value.
func TestSetGlobalsLeavesUnsetGlobalsAlone(t *testing.T) {
	SetGlobals(t, &config.Config{}, WithProfile("outer"), WithVerbose(true))

	t.Run("inner sets only DryRun", func(t *testing.T) {
		SetGlobals(t, &config.Config{}, WithDryRun(true))
		if Profile != "outer" {
			t.Errorf("Profile = %q, want %q — an unset option must not zero it", Profile, "outer")
		}
		if !Verbose {
			t.Error("Verbose = false, want true — an unset option must not zero it")
		}
	})
}

// TestSwapResolveTargetRestoresOnCleanup asserts the seam is swapped inside the
// subtest and restored afterwards, and that ResolveTarget routes through it.
func TestSwapResolveTargetRestoresOnCleanup(t *testing.T) {
	stub := &swapStubTarget{name: "stub"}

	t.Run("swapped", func(t *testing.T) {
		SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
			return stub, nil
		})

		got, err := ResolveTarget(context.Background(), &config.Config{}, "gamelift")
		if err != nil {
			t.Fatalf("ResolveTarget() error = %v, want nil", err)
		}
		if got != stub {
			t.Errorf("ResolveTarget() = %v, want the swapped stub", got)
		}
	})

	// After cleanup the real implementation is back, so an unknown target must be
	// rejected — the stub above accepted everything.
	if _, err := ResolveTarget(context.Background(), &config.Config{}, "definitely-not-a-target"); err == nil {
		t.Error("ResolveTarget() accepted an unknown target after cleanup; the seam was not restored")
	}
}

// TestSwapResolveTargetPropagatesError asserts an error from the swapped
// function reaches the caller unchanged.
func TestSwapResolveTargetPropagatesError(t *testing.T) {
	wantErr := errors.New("resolve exploded")

	SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, wantErr
	})

	_, err := ResolveTarget(context.Background(), &config.Config{}, "")
	if !errors.Is(err, wantErr) {
		t.Errorf("ResolveTarget() error = %v, want %v", err, wantErr)
	}
}

// swapStubTarget is a deploy.Target that records nothing; identity is the point.
type swapStubTarget struct {
	name string
}

func (s *swapStubTarget) Name() string                      { return s.name }
func (s *swapStubTarget) Capabilities() deploy.Capabilities { return deploy.Capabilities{} }
func (s *swapStubTarget) Deploy(context.Context, deploy.DeployInput) (*deploy.DeployResult, error) {
	return nil, nil
}
func (s *swapStubTarget) Status(context.Context) (*deploy.DeployStatus, error) { return nil, nil }
func (s *swapStubTarget) Destroy(context.Context) error                        { return nil }
