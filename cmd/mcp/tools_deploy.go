package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/pricing"
	"github.com/devrecon/ludus/internal/stack"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/tags"
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
	MaxPlayers int `json:"max_players,omitempty" jsonschema:"Maximum number of players for the game session (default: 8)"`
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
	Target string `json:"target,omitempty" jsonschema:"Deployment target to destroy: gamelift, stack, binary, anywhere, or ec2"`
}

type deployFleetResult struct {
	Success              bool    `json:"success"`
	FleetID              string  `json:"fleet_id,omitempty"`
	Status               string  `json:"status,omitempty"`
	Detail               string  `json:"detail,omitempty"`
	DurationSeconds      float64 `json:"duration_seconds,omitempty"`
	EstimatedCostPerHour float64 `json:"estimated_cost_per_hour,omitempty"`
	SessionID            string  `json:"session_id,omitempty"`
	SessionIP            string  `json:"session_ip,omitempty"`
	SessionPort          int     `json:"session_port,omitempty"`
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
	SessionID            string  `json:"session_id,omitempty"`
	SessionIP            string  `json:"session_ip,omitempty"`
	SessionPort          int     `json:"session_port,omitempty"`
	Output               string  `json:"output,omitempty"`
	Error                string  `json:"error,omitempty"`
}

type deployAnywhereResult struct {
	Success     bool   `json:"success"`
	FleetID     string `json:"fleet_id,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	Port        int    `json:"port,omitempty"`
	PID         int    `json:"pid,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	SessionIP   string `json:"session_ip,omitempty"`
	SessionPort int    `json:"session_port,omitempty"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
}

type deployEC2Result struct {
	Success              bool    `json:"success"`
	FleetID              string  `json:"fleet_id,omitempty"`
	BuildID              string  `json:"build_id,omitempty"`
	Status               string  `json:"status,omitempty"`
	DurationSeconds      float64 `json:"duration_seconds,omitempty"`
	EstimatedCostPerHour float64 `json:"estimated_cost_per_hour,omitempty"`
	SessionID            string  `json:"session_id,omitempty"`
	SessionIP            string  `json:"session_ip,omitempty"`
	SessionPort          int     `json:"session_port,omitempty"`
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
		Description: "Tear down deployed resources. Destroys fleet, container group definition, and IAM role.",
	}, handleDeployDestroy)
}

func handleDeployFleet(ctx context.Context, _ *mcp.CallToolRequest, input deployFleetInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	start := time.Now()

	// Apply overrides
	if input.Region != "" {
		cfg.AWS.Region = input.Region
	}
	if input.InstanceType != "" {
		cfg.GameLift.InstanceType = input.InstanceType
	}
	if input.FleetName != "" {
		cfg.GameLift.FleetName = input.FleetName
	}

	target, err := globals.ResolveTarget(ctx, cfg, "")
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve deploy target: %v", err))
	}

	serverBuildDir := resolveServerBuildDir(cfg)
	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, cfg.AWS.Region, cfg.AWS.ECRRepository, cfg.Container.Tag)

	var result deployFleetResult

	captured, err := withCapture(func() error {
		dr, deployErr := target.Deploy(ctx, deploy.DeployInput{
			ImageURI:       imageURI,
			ServerBuildDir: serverBuildDir,
			ServerPort:     cfg.Container.ServerPort,
		})
		if dr != nil {
			result.Status = dr.Status
			result.Detail = dr.Detail
		}
		return deployErr
	})
	result.Output = captured.Stdout + captured.Stderr
	result.DurationSeconds = time.Since(start).Seconds()

	if err != nil {
		result.Error = fmt.Sprintf("fleet deployment failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	result.Success = true
	if cost, ok := pricing.EstimateCost(cfg.GameLift.InstanceType); ok {
		result.EstimatedCostPerHour = cost
	}

	// Read fleet ID from state
	st, _ := state.Load()
	if st.Fleet != nil {
		result.FleetID = st.Fleet.FleetID
	}

	// Auto-session if requested
	if input.WithSession {
		sm, ok := target.(deploy.SessionManager)
		if ok {
			si, err := sm.CreateSession(ctx, 8)
			if err == nil && si != nil {
				result.SessionID = si.SessionID
				result.SessionIP = si.IPAddress
				result.SessionPort = si.Port
			}
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleDeployStack(ctx context.Context, _ *mcp.CallToolRequest, input deployStackInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	start := time.Now()

	// Apply overrides
	if input.Region != "" {
		cfg.AWS.Region = input.Region
	}
	if input.InstanceType != "" {
		cfg.GameLift.InstanceType = input.InstanceType
	}
	if input.FleetName != "" {
		cfg.GameLift.FleetName = input.FleetName
	}

	sn := input.StackName
	if sn == "" {
		sn = fmt.Sprintf("ludus-%s", cfg.GameLift.FleetName)
	}

	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, cfg.AWS.Region, cfg.AWS.ECRRepository, cfg.Container.Tag)

	awsCfg, err := gamelift.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return toolError(fmt.Sprintf("could not load AWS config: %v", err))
	}

	deployer := stack.NewStackDeployer(stack.StackOptions{
		StackName:          sn,
		Region:             cfg.AWS.Region,
		ImageURI:           imageURI,
		FleetName:          cfg.GameLift.FleetName,
		InstanceType:       cfg.GameLift.InstanceType,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		ServerSDKVersion:   "5.4.0",
		Tags:               tags.Build(cfg),
	}, awsCfg)

	var result deployStackResult
	result.StackName = sn

	captured, err := withCapture(func() error {
		sr, deployErr := deployer.Deploy(ctx)
		if sr != nil {
			result.Status = sr.Status
			result.FleetID = sr.FleetID
		}
		return deployErr
	})
	result.Output = captured.Stdout + captured.Stderr
	result.DurationSeconds = time.Since(start).Seconds()

	if err != nil {
		result.Error = fmt.Sprintf("stack deployment failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	result.Success = true
	if cost, ok := pricing.EstimateCost(cfg.GameLift.InstanceType); ok {
		result.EstimatedCostPerHour = cost
	}

	// Auto-session if requested
	if input.WithSession {
		adapter := stack.NewTargetAdapter(deployer)
		si, err := adapter.CreateSession(ctx, 8)
		if err == nil && si != nil {
			result.SessionID = si.SessionID
			result.SessionIP = si.IPAddress
			result.SessionPort = si.Port
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleDeployAnywhere(ctx context.Context, _ *mcp.CallToolRequest, input deployAnywhereInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	// Apply overrides
	if input.Region != "" {
		cfg.AWS.Region = input.Region
	}
	if input.FleetName != "" {
		cfg.GameLift.FleetName = input.FleetName
	}
	if input.IPAddress != "" {
		cfg.Anywhere.IPAddress = input.IPAddress
	}

	target, err := globals.ResolveTarget(ctx, cfg, "anywhere")
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve anywhere target: %v", err))
	}

	var result deployAnywhereResult

	captured, err := withCapture(func() error {
		dr, deployErr := target.Deploy(ctx, deploy.DeployInput{
			ServerPort: cfg.Container.ServerPort,
		})
		if dr != nil {
			result.FleetID = ""
			result.IPAddress = cfg.Anywhere.IPAddress
			result.Port = cfg.Container.ServerPort
		}
		_ = dr
		return deployErr
	})
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("anywhere deployment failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
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

	// Auto-session if requested
	if input.WithSession {
		sm, ok := target.(deploy.SessionManager)
		if ok {
			si, err := sm.CreateSession(ctx, 8)
			if err == nil && si != nil {
				result.SessionID = si.SessionID
				result.SessionIP = si.IPAddress
				result.SessionPort = si.Port
			}
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleDeployEC2(ctx context.Context, _ *mcp.CallToolRequest, input deployEC2Input) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	start := time.Now()

	// Apply overrides
	if input.Region != "" {
		cfg.AWS.Region = input.Region
	}
	if input.InstanceType != "" {
		cfg.GameLift.InstanceType = input.InstanceType
	}
	if input.FleetName != "" {
		cfg.GameLift.FleetName = input.FleetName
	}
	if input.Arch != "" {
		cfg.Game.Arch = input.Arch
	}

	target, err := globals.ResolveTarget(ctx, cfg, "ec2")
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve ec2 target: %v", err))
	}

	serverBuildDir := resolveServerBuildDir(cfg)
	var result deployEC2Result

	captured, err := withCapture(func() error {
		dr, deployErr := target.Deploy(ctx, deploy.DeployInput{
			ServerBuildDir: serverBuildDir,
			ServerPort:     cfg.Container.ServerPort,
		})
		if dr != nil {
			result.Status = dr.Status
		}
		return deployErr
	})
	result.Output = captured.Stdout + captured.Stderr
	result.DurationSeconds = time.Since(start).Seconds()

	if err != nil {
		result.Error = fmt.Sprintf("EC2 fleet deployment failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	result.Success = true
	if cost, ok := pricing.EstimateCost(cfg.GameLift.InstanceType); ok {
		result.EstimatedCostPerHour = cost
	}

	// Read state for fleet/build details
	st, _ := state.Load()
	if st.EC2Fleet != nil {
		result.FleetID = st.EC2Fleet.FleetID
		result.BuildID = st.EC2Fleet.BuildID
	}

	// Auto-session if requested
	if input.WithSession {
		sm, ok := target.(deploy.SessionManager)
		if ok {
			si, err := sm.CreateSession(ctx, 8)
			if err == nil && si != nil {
				result.SessionID = si.SessionID
				result.SessionIP = si.IPAddress
				result.SessionPort = si.Port
			}
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleDeploySession(ctx context.Context, _ *mcp.CallToolRequest, input deploySessionInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	target, err := globals.ResolveTarget(ctx, cfg, "")
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve deploy target: %v", err))
	}

	sm, ok := target.(deploy.SessionManager)
	if !ok {
		return toolError(fmt.Sprintf("target %q does not support game sessions", target.Name()))
	}

	maxPlayers := input.MaxPlayers
	if maxPlayers <= 0 {
		maxPlayers = 8
	}

	var result deploySessionResult

	captured, err := withCapture(func() error {
		si, sessionErr := sm.CreateSession(ctx, maxPlayers)
		if si != nil {
			result.SessionID = si.SessionID
			result.IPAddress = si.IPAddress
			result.Port = si.Port
		}
		return sessionErr
	})
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("session creation failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	result.Success = true
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleDeployDestroy(ctx context.Context, _ *mcp.CallToolRequest, input deployDestroyInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	target, err := globals.ResolveTarget(ctx, cfg, input.Target)
	if err != nil {
		return toolError(fmt.Sprintf("could not resolve deploy target: %v", err))
	}

	var result deployDestroyResult

	captured, err := withCapture(func() error {
		return target.Destroy(ctx)
	})
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("destroy failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	// Clear fleet state
	_ = state.ClearFleet()

	result.Success = true
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}
