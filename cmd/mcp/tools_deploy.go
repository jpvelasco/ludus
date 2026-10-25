package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/cleanup"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/ec2fleet"
	"github.com/jpvelasco/ludus/internal/pricing"
	"github.com/jpvelasco/ludus/internal/stack"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type deployFleetInput struct {
	Region       string `json:"region,omitempty" jsonschema:"AWS region override"`
	InstanceType string `json:"instance_type,omitempty" jsonschema:"EC2 instance type override"`
	FleetName    string `json:"fleet_name,omitempty" jsonschema:"GameLift fleet name override"`
	WithSession  bool   `json:"with_session,omitempty" jsonschema:"Create a game session after deployment"`
	DryRun       bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type deploySessionInput struct {
	MaxPlayers int  `json:"max_players,omitempty" jsonschema:"Maximum number of players for the game session (default: 8)"`
	DryRun     bool `json:"dry_run,omitempty" jsonschema:"Print the session plan without creating a session"`
}

type deployStackInput struct {
	Region       string `json:"region,omitempty" jsonschema:"AWS region override"`
	InstanceType string `json:"instance_type,omitempty" jsonschema:"EC2 instance type override"`
	FleetName    string `json:"fleet_name,omitempty" jsonschema:"GameLift fleet name override"`
	StackName    string `json:"stack_name,omitempty" jsonschema:"CloudFormation stack name override"`
	WithSession  bool   `json:"with_session,omitempty" jsonschema:"Create a game session after deployment"`
	DryRun       bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type deployAnywhereInput struct {
	Region      string `json:"region,omitempty" jsonschema:"AWS region override"`
	FleetName   string `json:"fleet_name,omitempty" jsonschema:"GameLift fleet name override"`
	IPAddress   string `json:"ip_address,omitempty" jsonschema:"Local IP address override (default: auto-detect)"`
	WithSession bool   `json:"with_session,omitempty" jsonschema:"Create a game session after deployment"`
	DryRun      bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type deployEC2Input struct {
	Region       string `json:"region,omitempty" jsonschema:"AWS region override"`
	InstanceType string `json:"instance_type,omitempty" jsonschema:"EC2 instance type override"`
	FleetName    string `json:"fleet_name,omitempty" jsonschema:"GameLift fleet name override"`
	Arch         string `json:"arch,omitempty" jsonschema:"Target CPU architecture: amd64 or arm64 (default: from config)"`
	WithSession  bool   `json:"with_session,omitempty" jsonschema:"Create a game session after deployment"`
	DryRun       bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type deployDestroyInput struct {
	Target     string `json:"target,omitempty" jsonschema:"Deployment target to destroy: gamelift, stack, binary, anywhere, or ec2"`
	AllTargets bool   `json:"all_targets,omitempty" jsonschema:"Tear down every deploy target, not just the active/specified one. Still preserves durable artifacts unless purge is also set."`
	Purge      bool   `json:"purge,omitempty" jsonschema:"Also delete durable build artifacts (ECR repositories and S3 build buckets). Irreversible; runs without a confirmation prompt in MCP, so set deliberately."`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"Print the teardown plan without destroying resources"`
}

type deployFleetResult struct {
	Success              bool    `json:"success"`
	FleetID              string  `json:"fleet_id,omitempty"`
	Status               string  `json:"status,omitempty"`
	Detail               string  `json:"detail,omitempty"`
	DurationSeconds      float64 `json:"duration_seconds,omitempty"`
	EstimatedCostPerHour float64 `json:"estimated_cost_per_hour,omitempty"`
	InstanceGuidance     string  `json:"instance_guidance,omitempty"`
	SessionID            string  `json:"session_id,omitempty"`
	SessionIP            string  `json:"session_ip,omitempty"`
	SessionPort          int     `json:"session_port,omitempty"`
	SessionError         string  `json:"session_error,omitempty"`
	Output               string  `json:"output,omitempty"`
	Error                string  `json:"error,omitempty"`
}

type deploySessionResult struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	Port      int    `json:"port,omitempty"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

type deployStackResult struct {
	Success              bool    `json:"success"`
	StackName            string  `json:"stack_name,omitempty"`
	FleetID              string  `json:"fleet_id,omitempty"`
	Status               string  `json:"status,omitempty"`
	DurationSeconds      float64 `json:"duration_seconds,omitempty"`
	EstimatedCostPerHour float64 `json:"estimated_cost_per_hour,omitempty"`
	InstanceGuidance     string  `json:"instance_guidance,omitempty"`
	SessionID            string  `json:"session_id,omitempty"`
	SessionIP            string  `json:"session_ip,omitempty"`
	SessionPort          int     `json:"session_port,omitempty"`
	SessionError         string  `json:"session_error,omitempty"`
	Output               string  `json:"output,omitempty"`
	Error                string  `json:"error,omitempty"`
}

type deployAnywhereResult struct {
	Success      bool   `json:"success"`
	FleetID      string `json:"fleet_id,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
	Port         int    `json:"port,omitempty"`
	PID          int    `json:"pid,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	SessionIP    string `json:"session_ip,omitempty"`
	SessionPort  int    `json:"session_port,omitempty"`
	SessionError string `json:"session_error,omitempty"`
	Output       string `json:"output,omitempty"`
	Error        string `json:"error,omitempty"`
}

type deployEC2Result struct {
	Success              bool    `json:"success"`
	FleetID              string  `json:"fleet_id,omitempty"`
	BuildID              string  `json:"build_id,omitempty"`
	Status               string  `json:"status,omitempty"`
	DurationSeconds      float64 `json:"duration_seconds,omitempty"`
	EstimatedCostPerHour float64 `json:"estimated_cost_per_hour,omitempty"`
	InstanceGuidance     string  `json:"instance_guidance,omitempty"`
	SessionID            string  `json:"session_id,omitempty"`
	SessionIP            string  `json:"session_ip,omitempty"`
	SessionPort          int     `json:"session_port,omitempty"`
	SessionError         string  `json:"session_error,omitempty"`
	Output               string  `json:"output,omitempty"`
	Error                string  `json:"error,omitempty"`
}

type deployDestroyResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

func registerDeployTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_deploy_fleet",
		Description: "Deploy a GameLift container fleet. Creates container group definition, IAM role, and fleet. This is a long-running operation (can take 15-30 minutes).",
	}, handleDeployFleet)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_deploy_stack",
		Description: "Deploy a CloudFormation stack that provisions GameLift resources (IAM role, container group definition, fleet). Atomic with automatic rollback. This is a long-running operation.",
	}, handleDeployStack)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_deploy_anywhere",
		Description: "Deploy a GameLift Anywhere fleet on the local machine. Creates fleet, registers compute, and launches game server locally. Fast iteration — fleet creation takes seconds.",
	}, handleDeployAnywhere)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_deploy_ec2",
		Description: "Deploy a GameLift Managed EC2 fleet. Uploads server build to S3, creates GameLift build, and provisions EC2 fleet. No Docker required. This is a long-running operation.",
	}, handleDeployEC2)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_deploy_session",
		Description: "Create a game session on the deployed fleet. Returns connection details (IP address and port).",
	}, handleDeploySession)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_deploy_destroy",
		Description: "Tear down deployed resources for the active target (fleet, container group definition, IAM role, EC2 build + its S3 object). Durable artifacts (ECR repositories, S3 build buckets) are preserved by default. Set all_targets=true to sweep every target; set purge=true to also delete the durable artifacts (irreversible).",
	}, handleDeployDestroy)
}

// routeTargetOutput points a deploy target's deployer output at the
// per-operation capture when the target supports it (#765). Targets that
// don't implement deploy.OutputSetter keep their default global streams.
func routeTargetOutput(target deploy.Target, out *toolOutput) {
	if setter, ok := target.(deploy.OutputSetter); ok {
		setter.SetOutput(out)
	}
}

func handleDeployFleet(ctx context.Context, _ *mcp.CallToolRequest, input deployFleetInput) (*mcp.CallToolResult, any, error) {
	if deployToolDryRun(input.DryRun) {
		return resultOK(deployFleetResult{Success: true, Output: dryRunOutput("deploy a GameLift container fleet")})
	}
	cfg := isolatedConfig(deployOverrides{Region: input.Region, InstanceType: input.InstanceType, FleetName: input.FleetName})
	start := time.Now()

	target, err := globals.ResolveTarget(ctx, &cfg, "gamelift")
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve deploy target: %v", err))
	}

	out := newToolOutput()
	routeTargetOutput(target, out)

	serverBuildDir := config.ResolveServerBuildDir(&cfg)
	env, err := awsenv.NewResolver(globals.DryRun).Resolve(ctx, &cfg, awsenv.Requirements{Account: true, Region: true})
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve AWS environment: %v", err))
	}
	imageURI, err := awsenv.ImageURI(env, cfg.AWS.ECRRepository, cfg.Container.Tag)
	if err != nil {
		return toolError(fmt.Sprintf("could not build ECR image URI: %v", err))
	}

	var result deployFleetResult

	dr, deployErr := target.Deploy(ctx, deploy.DeployInput{
		ImageURI:       imageURI,
		ServerBuildDir: serverBuildDir,
		ServerPort:     cfg.Container.ServerPort,
	})
	if dr != nil {
		result.Status = dr.Status
		result.Detail = dr.Detail
	}
	err = deployErr
	result.DurationSeconds = time.Since(start).Seconds()
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("fleet deployment failed: %v", err)
		return resultErr(result)
	}

	result.Success = true
	ci := estimateCost(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch())
	result.EstimatedCostPerHour = ci.EstimatedCostPerHour
	result.InstanceGuidance = ci.InstanceGuidance

	// Read fleet ID from state
	st, _ := state.Load()
	if st.Fleet != nil {
		result.FleetID = st.Fleet.FleetID
	}

	tryCreateSession(ctx, target, input.WithSession, &result)

	return resultOK(result)
}

func handleDeployStack(ctx context.Context, _ *mcp.CallToolRequest, input deployStackInput) (*mcp.CallToolResult, any, error) {
	if deployToolDryRun(input.DryRun) {
		return resultOK(deployStackResult{Success: true, Output: dryRunOutput("deploy a CloudFormation stack")})
	}
	cfg := isolatedConfig(deployOverrides{Region: input.Region, InstanceType: input.InstanceType, FleetName: input.FleetName})
	start := time.Now()
	out := newToolOutput()

	// Auto-default instance type based on server architecture
	if resolved, switched := pricing.AutoSwitch(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch()); switched {
		out.Printf("Note: Switching instance type from %s to %s to match %s server architecture\n",
			cfg.GameLift.InstanceType, resolved, cfg.Game.ResolvedArch())
		cfg.GameLift.InstanceType = resolved
	}

	sn := input.StackName
	if sn == "" {
		sn = fmt.Sprintf("ludus-%s", cfg.GameLift.FleetName)
	}

	env, err := awsenv.NewResolver(globals.DryRun).Resolve(ctx, &cfg, awsenv.Requirements{Account: true, Region: true})
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve AWS environment: %v", err))
	}
	imageURI, err := awsenv.ImageURI(env, cfg.AWS.ECRRepository, cfg.Container.Tag)
	if err != nil {
		return toolError(fmt.Sprintf("could not build ECR image URI: %v", err))
	}

	deployer := stack.NewStackDeployer(globals.BuildStackOptions(&cfg, env, imageURI, sn, cfg.GameLift.FleetName), env.AWSConfig)
	target := stack.NewTargetAdapter(deployer)
	routeTargetOutput(target, out)

	var result deployStackResult
	result.StackName = sn

	sr, deployErr := deployer.Deploy(ctx)
	if sr != nil {
		result.Status = sr.Status
		result.FleetID = sr.FleetID
	}
	err = deployErr
	result.DurationSeconds = time.Since(start).Seconds()
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("stack deployment failed: %v", err)
		return resultErr(result)
	}

	result.Success = true
	ci := estimateCost(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch())
	result.EstimatedCostPerHour = ci.EstimatedCostPerHour
	result.InstanceGuidance = ci.InstanceGuidance

	tryCreateSession(ctx, target, input.WithSession, &result)

	return resultOK(result)
}

func handleDeployAnywhere(ctx context.Context, _ *mcp.CallToolRequest, input deployAnywhereInput) (*mcp.CallToolResult, any, error) {
	if deployToolDryRun(input.DryRun) {
		return resultOK(deployAnywhereResult{Success: true, Output: dryRunOutput("deploy a GameLift Anywhere fleet")})
	}
	cfg := isolatedConfig(deployOverrides{Region: input.Region, FleetName: input.FleetName, IPAddress: input.IPAddress})

	target, err := globals.ResolveTarget(ctx, &cfg, "anywhere")
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve anywhere target: %v", err))
	}

	out := newToolOutput()
	routeTargetOutput(target, out)

	var result deployAnywhereResult

	dr, err := target.Deploy(ctx, deploy.DeployInput{
		ServerPort: cfg.Container.ServerPort,
	})
	if dr != nil {
		result.FleetID = ""
		result.IPAddress = cfg.Anywhere.IPAddress
		result.Port = cfg.Container.ServerPort
	}
	_ = dr
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("anywhere deployment failed: %v", err)
		return resultErr(result)
	}

	result.Success = true

	// Read state for fleet/PID details
	st, _ := state.Load()
	if st.Anywhere != nil {
		result.FleetID = st.Anywhere.FleetID
		result.IPAddress = st.Anywhere.IPAddress
		result.Port = st.Anywhere.ServerPort
		result.PID = st.Anywhere.PID
	}

	tryCreateSession(ctx, target, input.WithSession, &result)

	return resultOK(result)
}

func handleDeployEC2(ctx context.Context, _ *mcp.CallToolRequest, input deployEC2Input) (*mcp.CallToolResult, any, error) {
	if deployToolDryRun(input.DryRun) {
		return resultOK(deployEC2Result{Success: true, Output: dryRunOutput("deploy a GameLift Managed EC2 fleet")})
	}
	cfg := isolatedConfig(deployOverrides{Region: input.Region, InstanceType: input.InstanceType, FleetName: input.FleetName, Arch: input.Arch})
	start := time.Now()

	target, err := globals.ResolveTarget(ctx, &cfg, "ec2")
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve ec2 target: %v", err))
	}

	out := newToolOutput()
	routeTargetOutput(target, out)

	serverBuildDir := config.ResolveServerBuildDir(&cfg)
	var result deployEC2Result

	dr, deployErr := target.Deploy(ctx, deploy.DeployInput{
		ServerBuildDir: serverBuildDir,
		ServerPort:     cfg.Container.ServerPort,
	})
	if dr != nil {
		result.Status = dr.Status
	}
	err = deployErr
	result.DurationSeconds = time.Since(start).Seconds()
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("EC2 fleet deployment failed: %v", err)
		return resultErr(result)
	}

	result.Success = true
	ci := estimateCost(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch())
	result.EstimatedCostPerHour = ci.EstimatedCostPerHour
	result.InstanceGuidance = ci.InstanceGuidance

	// Read state for fleet/build details
	st, _ := state.Load()
	if st.EC2Fleet != nil {
		result.FleetID = st.EC2Fleet.FleetID
		result.BuildID = st.EC2Fleet.BuildID
	}

	tryCreateSession(ctx, target, input.WithSession, &result)

	return resultOK(result)
}

func handleDeploySession(ctx context.Context, _ *mcp.CallToolRequest, input deploySessionInput) (*mcp.CallToolResult, any, error) {
	if deployToolDryRun(input.DryRun) {
		return resultOK(deploySessionResult{Success: true, Output: dryRunOutput("create a game session")})
	}
	cfg := globals.Cfg

	target, err := globals.ResolveSessionTarget(ctx, cfg)
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve deploy target: %v", err))
	}

	sm, ok := target.(deploy.SessionManager)
	if !ok {
		return toolError(fmt.Sprintf("target %q does not support game sessions", target.Name()))
	}

	out := newToolOutput()
	routeTargetOutput(target, out)

	maxPlayers := input.MaxPlayers
	if maxPlayers <= 0 {
		maxPlayers = 8
	}

	var result deploySessionResult

	si, err := sm.CreateSession(ctx, maxPlayers)
	if si != nil {
		result.SessionID = si.SessionID
		result.IPAddress = si.IPAddress
		result.Port = si.Port
	}
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("session creation failed: %v", err)
		return resultErr(result)
	}

	result.Success = true
	return resultOK(result)
}

func handleDeployDestroy(ctx context.Context, _ *mcp.CallToolRequest, input deployDestroyInput) (*mcp.CallToolResult, any, error) {
	if deployToolDryRun(input.DryRun) {
		return resultOK(deployDestroyResult{Success: true, Output: dryRunOutput("destroy deployed resources")})
	}
	cfg := globals.Cfg
	var result deployDestroyResult

	out := newToolOutput()
	err := runDestroyForMCP(ctx, cfg, input, out)
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("destroy failed: %v", err)
		return resultErr(result)
	}

	result.Success = true
	return resultOK(result)
}

// runDestroyForMCP mirrors the CLI destroy scoping: default tears down only the
// active target's ephemeral resources; all_targets sweeps every target; purge
// also deletes durable artifacts (ECR repos + S3 build buckets). MCP is
// non-interactive, so purge runs without a confirmation prompt. Sweep status
// prints route through out so the response carries the transcript (#725,
// #747); resolved targets that support output routing join the same capture
// (#765).
func runDestroyForMCP(ctx context.Context, cfg *config.Config, input deployDestroyInput, out *toolOutput) error {
	var sweepErr error
	if input.AllTargets {
		sweepErr = destroyAllTargets(ctx, cfg, out)
	} else {
		target, err := globals.ResolveTarget(ctx, cfg, input.Target)
		if err != nil {
			return fmt.Errorf("could not resolve deploy target: %w", err)
		}
		routeTargetOutput(target, out)
		if err := target.Destroy(ctx); err != nil {
			return err
		}
	}

	if input.Purge {
		return errors.Join(sweepErr, cleanupSharedResources(ctx, cfg, out))
	}
	return sweepErr
}

// destroyAllTargets attempts to destroy every known deploy target, reporting
// and collecting each failure so the sweep continues; it returns an aggregate
// error when any target failed to tear down.
func destroyAllTargets(ctx context.Context, cfg *config.Config, out *toolOutput) error {
	var failures []string
	for _, name := range []string{"gamelift", "stack", "ec2", "anywhere", "binary"} {
		target, err := globals.ResolveTarget(ctx, cfg, name)
		if err != nil {
			continue
		}
		routeTargetOutput(target, out)
		if err := target.Destroy(ctx); err != nil {
			out.Printf("  %s: %v (continuing)\n", name, err)
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("destroy sweep failed for %d target(s): %s", len(failures), strings.Join(failures, "; "))
	}
	return nil
}

// cleanupSharedResources deletes ECR repositories and S3 buckets, surfacing
// configuration and deletion failures as errors so the MCP response reflects
// a failed purge.
func cleanupSharedResources(ctx context.Context, cfg *config.Config, out *toolOutput) error {
	awsCfg, err := awsutil.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	cl := cleanup.NewCleaner(awsCfg)
	var failures []string
	if err := cleanupECRRepos(ctx, cl, cfg, out); err != nil {
		failures = append(failures, err.Error())
	}
	if err := cleanupS3Bucket(ctx, cl, awsCfg, cfg, out); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return fmt.Errorf("purge failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

// cleanupECRRepos deletes the game server ECR repository.
func cleanupECRRepos(ctx context.Context, cl cleanup.Deleter, cfg *config.Config, out *toolOutput) error {
	ecrRepo := cfg.AWS.ECRRepository
	if ecrRepo == "" {
		ecrRepo = "ludus-server"
	}
	if err := cl.DeleteECRRepository(ctx, ecrRepo); err != nil {
		out.Printf("  ECR %s: %v (continuing)\n", ecrRepo, err)
		return fmt.Errorf("ECR repository %s: %w", ecrRepo, err)
	}
	return nil
}

// cleanupS3Bucket deletes the ludus builds S3 bucket.
func cleanupS3Bucket(ctx context.Context, cl cleanup.Deleter, awsCfg aws.Config, cfg *config.Config, out *toolOutput) error {
	accountID := cfg.AWS.AccountID
	if accountID == "" {
		stsClient := sts.NewFromConfig(awsCfg)
		id, err := awsenv.AccountID(ctx, stsClient)
		if err != nil {
			out.Printf("  Warning: could not resolve account ID: %v\n", err)
		} else {
			accountID = id
		}
	}
	if accountID == "" {
		return nil
	}
	bucket := cfg.EC2Fleet.S3Bucket
	if bucket == "" {
		bucket = ec2fleet.BuildBucketName(accountID)
	}
	if err := cl.DeleteS3Bucket(ctx, bucket); err != nil {
		out.Printf("  S3 %s: %v (continuing)\n", bucket, err)
		return fmt.Errorf("S3 bucket %s: %w", bucket, err)
	}
	return nil
}
