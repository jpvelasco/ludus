package mcp

import (
	"context"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/pricing"
	"github.com/devrecon/ludus/internal/runner"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionReceiver is implemented by MCP result types that can receive session info.
type sessionReceiver interface {
	setSession(id, ip string, port int)
}

func (r *deployFleetResult) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

func (r *deployStackResult) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

func (r *deployAnywhereResult) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

func (r *deployEC2Result) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

// tryCreateSession creates a game session via the target's SessionManager if
// withSession is true. Session info is written to the result via sessionReceiver.
func tryCreateSession(ctx context.Context, target deploy.Target, withSession bool, result sessionReceiver) {
	if !withSession {
		return
	}
	sm, ok := target.(deploy.SessionManager)
	if !ok {
		return
	}
	si, err := sm.CreateSession(ctx, 8)
	if err == nil && si != nil {
		result.setSession(si.SessionID, si.IPAddress, si.Port)
	}
}

// checkCacheHit returns an early MCP result if the cache stage is up to date.
// Returns nil if cache missed or disabled. The cachedResult should be a struct
// with Success/Output fields that serializes to JSON (e.g. gameBuildResult, engineResult).
func checkCacheHit(noCache bool, stage cache.StageKey, hash string, cachedResult any) *mcpsdk.CallToolResult {
	if noCache {
		return nil
	}
	c, err := cache.Load()
	if err != nil {
		return nil
	}
	if !c.IsHit(stage, hash) {
		return nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: jsonString(cachedResult)}},
	}
}

// --- Result constructors ---

// resultOK returns a successful CallToolResult with the given value serialized as JSON.
func resultOK(v any) (*mcpsdk.CallToolResult, any, error) {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: jsonString(v)},
		},
	}, nil, nil
}

// resultErr returns an error CallToolResult with the given value serialized as JSON.
func resultErr(v any) (*mcpsdk.CallToolResult, any, error) {
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: jsonString(v)},
		},
	}, nil, nil
}

// --- Runner helper ---

// newToolRunner creates a runner configured for MCP tool handlers.
// Verbose is always true (MCP captures output); dryRun respects both input and global flags.
func newToolRunner(dryRun bool) *runner.Runner {
	return runner.NewRunner(true, dryRun || globals.DryRun)
}

// --- Config override helpers ---

// applyRegionOverride sets the AWS region on cfg if region is non-empty.
func applyRegionOverride(cfg *config.Config, region string) {
	if region != "" {
		cfg.AWS.Region = region
	}
}

// applyInstanceOverride sets the GameLift instance type on cfg if instanceType is non-empty.
func applyInstanceOverride(cfg *config.Config, instanceType string) {
	if instanceType != "" {
		cfg.GameLift.InstanceType = instanceType
	}
}

// applyFleetNameOverride sets the GameLift fleet name on cfg if fleetName is non-empty.
func applyFleetNameOverride(cfg *config.Config, fleetName string) {
	if fleetName != "" {
		cfg.GameLift.FleetName = fleetName
	}
}

// applyArchOverride sets the game architecture on cfg if arch is non-empty.
func applyArchOverride(cfg *config.Config, arch string) {
	if arch != "" {
		cfg.Game.Arch = arch
	}
}

// --- Backend helper ---

// resolveBackend returns the backend from the input, falling back to the config default.
func resolveBackend(inputBackend, configBackend string) string {
	if inputBackend != "" {
		return inputBackend
	}
	return configBackend
}

// --- Capture helper ---

// mergeOutput concatenates captured stdout and stderr.
func mergeOutput(c capturedOutput) string {
	return c.Stdout + c.Stderr
}

// saveCache persists a cache entry for the given stage and hash.
// Delegates to cache.RecordBuild for centralized cache management.
var saveCache = cache.RecordBuild

// --- Pricing helpers ---

// costInfo holds pricing estimation and instance guidance for deploy results.
type costInfo struct {
	EstimatedCostPerHour float64
	InstanceGuidance     string
}

// estimateCost returns pricing info for the given instance type and architecture.
func estimateCost(instanceType, arch string) costInfo {
	var info costInfo
	if cost, ok := pricing.EstimateCost(instanceType); ok {
		info.EstimatedCostPerHour = cost
	}
	info.InstanceGuidance = pricing.FormatGuidance(instanceType, arch)
	return info
}
