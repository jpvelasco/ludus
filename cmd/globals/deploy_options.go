package globals

import (
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ec2fleet"
	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/tags"
)

// BuildGameLiftOptions assembles effective options without AWS or runner construction.
func BuildGameLiftOptions(cfg *config.Config, region string, imageURI string) gamelift.DeployOptions {
	return gamelift.DeployOptions{
		Region:                 region,
		OwnershipTagKey:        cfg.GameLift.OwnershipTagKey,
		ImageURI:               imageURI,
		FleetName:              cfg.GameLift.FleetName,
		InstanceType:           cfg.GameLift.InstanceType,
		ContainerGroupName:     cfg.GameLift.ContainerGroupName,
		ServerPort:             cfg.Container.ServerPort,
		MaxConcurrentSessions:  cfg.GameLift.MaxConcurrentSessions,
		Tags:                   tags.Build(cfg),
		AllowedCIDR:            cfg.GameLift.AllowedCIDR,
		IAMRoleName:            cfg.GameLift.IAM.RoleName,
		IAMPath:                cfg.GameLift.IAM.Path,
		IAMPermissionsBoundary: cfg.GameLift.IAM.PermissionsBoundary,
	}
}

// buildEC2Options assembles effective options without AWS or runner construction.
func buildEC2Options(cfg *config.Config, region string) ec2fleet.DeployOptions {
	return ec2fleet.DeployOptions{
		Region:                region,
		FleetName:             cfg.GameLift.FleetName,
		InstanceType:          cfg.GameLift.InstanceType,
		ServerPort:            cfg.Container.ServerPort,
		MaxConcurrentSessions: cfg.GameLift.MaxConcurrentSessions,
		S3Bucket:              cfg.EC2Fleet.S3Bucket,
		ProjectName:           cfg.Game.ProjectName,
		PackagedDirName:       cfg.Game.ResolvedPackagedDirName(),
		ServerTarget:          cfg.Game.ResolvedServerTarget(),
		ServerMap:             cfg.Game.ServerMap,
		Arch:                  cfg.Game.ResolvedArch(),
		AllowedCIDR:           config.ResolvedAllowedCIDR(cfg.EC2Fleet.AllowedCIDR),
		Tags:                  tags.Build(cfg),
	}
}
