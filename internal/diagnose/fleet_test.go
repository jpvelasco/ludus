package diagnose

import (
	"errors"
	"strings"
	"testing"
)

func TestFleetRoleConstraintHintOverridesGenericAdvice(t *testing.T) {
	cause := errors.New("InvalidRequestException: 'fleetRoleArn' failed to satisfy constraint")
	err := DeployError(cause, "gamelift")
	for _, want := range []string{"root-path IAM role", "gamelift.iam.path", "empty or /"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "verify instance type") {
		t.Fatalf("incorrect constraint guidance: %v", err)
	}
}

func TestCapacityFallbackHint(t *testing.T) {
	err := DeployError(errors.New("FleetCapacityExceededException"), "gamelift")
	if !strings.Contains(err.Error(), "wait for a session to end") {
		t.Fatalf("missing capacity hint: %v", err)
	}
}

// awsDefinitionInUse mirrors the failure when a re-deploy with a new image tries
// to replace a container group definition a live fleet still references (#853).
const awsDefinitionInUse = "existing container group definition does not match desired config and could not be removed: " +
	"operation error GameLift: DeleteContainerGroupDefinition, https response error StatusCode: 400, " +
	"InvalidRequestException: ContainerGroupDefinition [g] cannot be deleted until all fleets and deployments " +
	"that are using the containerGroupDefinition are deleted: [fleet-1]"

func TestDefinitionInUseHintOverridesGenericAdvice(t *testing.T) {
	cause := errors.New(awsDefinitionInUse)
	err := DeployError(cause, "gamelift")
	for _, want := range []string{"in-place", "ludus deploy destroy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "verify instance type") {
		t.Fatalf("generic fleet-configuration advice must not mask the in-use cause: %v", err)
	}
}

func TestInvalidRequestWithoutDefinitionContextKeepsGenericHint(t *testing.T) {
	err := DeployError(errors.New("InvalidRequestException: unsupported instance type"), "gamelift")
	if !strings.Contains(err.Error(), "verify instance type") {
		t.Fatalf("other InvalidRequestException failures keep the generic hint: %v", err)
	}
}
