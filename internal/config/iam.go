package config

import "fmt"

// ValidateContainerIAMPath rejects paths that GameLift's FleetRoleArn
// constraint cannot accept, before a container group or role is created.
func ValidateContainerIAMPath(path string) error {
	if path != "" && path != "/" {
		return fmt.Errorf("gamelift.iam.path %q is unsupported: GameLift container fleets require a root-path role; use an empty path or /", path)
	}
	return nil
}
