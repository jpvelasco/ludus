package globals

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

// GlobalOption configures SetGlobals behavior.
type GlobalOption func(*globalConfig)

// globalConfig holds configuration for SetGlobals.
type globalConfig struct {
	verbose       *bool
	dryRun        *bool
	jsonOutput    *bool
	profile       *string
	ddcMode       *string
	showAccountID *bool
	noLogs        *bool
	commandName   *string
}

// WithVerbose enables verbose output.
func WithVerbose(v bool) GlobalOption {
	return func(gc *globalConfig) {
		gc.verbose = &v
	}
}

// WithDryRun enables dry-run mode.
func WithDryRun(v bool) GlobalOption {
	return func(gc *globalConfig) {
		gc.dryRun = &v
	}
}

// WithJSONOutput enables JSON output mode.
func WithJSONOutput(v bool) GlobalOption {
	return func(gc *globalConfig) {
		gc.jsonOutput = &v
	}
}

// WithProfile sets the profile name.
func WithProfile(v string) GlobalOption {
	return func(gc *globalConfig) {
		gc.profile = &v
	}
}

// WithDDCMode sets the DDC mode.
func WithDDCMode(v string) GlobalOption {
	return func(gc *globalConfig) {
		gc.ddcMode = &v
	}
}

// WithShowAccountID enables account ID display.
func WithShowAccountID(v bool) GlobalOption {
	return func(gc *globalConfig) {
		gc.showAccountID = &v
	}
}

// WithNoLogs disables build logging.
func WithNoLogs(v bool) GlobalOption {
	return func(gc *globalConfig) {
		gc.noLogs = &v
	}
}

// WithCommandName sets the command name.
func WithCommandName(v string) GlobalOption {
	return func(gc *globalConfig) {
		gc.commandName = &v
	}
}

// SetGlobals sets package globals for the duration of t and restores them in t.Cleanup.
// Do not use t.Parallel() in tests that call SetGlobals.
func SetGlobals(t *testing.T, cfg *config.Config, opts ...GlobalOption) {
	t.Helper()

	gc := &globalConfig{}
	for _, opt := range opts {
		opt(gc)
	}

	oldState := saveGlobalState()
	Cfg = cfg
	applyOptions(gc)
	resetBuildLogOnce()

	t.Cleanup(func() {
		restoreGlobalState(oldState)
		resetBuildLogOnce()
	})
}

// applyOptions applies GlobalOptions to globals.
func applyOptions(gc *globalConfig) {
	// Use explicit assignments instead of a loop to keep complexity low.
	// Ordered by: bools first, then strings.
	if gc.verbose != nil {
		Verbose = *gc.verbose
	}
	if gc.dryRun != nil {
		DryRun = *gc.dryRun
	}
	if gc.jsonOutput != nil {
		JSONOutput = *gc.jsonOutput
	}
	if gc.showAccountID != nil {
		ShowAccountID = *gc.showAccountID
	}
	if gc.noLogs != nil {
		NoLogs = *gc.noLogs
	}
	applyStringOptions(gc)
}

// applyStringOptions applies string GlobalOptions to globals.
func applyStringOptions(gc *globalConfig) {
	if gc.profile != nil {
		Profile = *gc.profile
	}
	if gc.ddcMode != nil {
		DDCMode = *gc.ddcMode
	}
	if gc.commandName != nil {
		CommandName = *gc.commandName
	}
}

// globalState holds the saved state of all globals.
type globalState struct {
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

// saveGlobalState saves the current global state.
func saveGlobalState() *globalState {
	return &globalState{
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

// restoreGlobalState restores globals from saved state.
func restoreGlobalState(old *globalState) {
	Cfg = old.cfg
	Verbose = old.verbose
	DryRun = old.dryRun
	JSONOutput = old.jsonOutput
	Profile = old.profile
	DDCMode = old.ddcMode
	ShowAccountID = old.showAccount
	NoLogs = old.noLogs
	CommandName = old.commandName
}

// SwapResolveTarget temporarily replaces ResolveTargetFn for testing.
// Must be restored via t.Cleanup.
func SwapResolveTarget(t *testing.T, fn func(context.Context, *config.Config, string) (deploy.Target, error)) {
	t.Helper()
	old := ResolveTargetFn
	ResolveTargetFn = fn
	t.Cleanup(func() {
		ResolveTargetFn = old
	})
}
