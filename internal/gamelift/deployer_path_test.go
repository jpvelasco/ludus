package gamelift

import (
	"context"
	"strings"
	"testing"
)

func TestContainerCreationRejectsIAMPathBeforeAWS(t *testing.T) {
	// Nil clients prove invalid paths are rejected before either create API.
	d := &Deployer{opts: DeployOptions{IAMPath: "/project/"}}
	for name, call := range map[string]func() error{
		"group": func() error {
			_, err := d.CreateContainerGroupDefinition(context.Background())
			return err
		},
		"fleet": func() error {
			_, err := d.CreateFleet(context.Background(), "group")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "root-path role") {
				t.Fatalf("create error = %v", err)
			}
		})
	}
}
