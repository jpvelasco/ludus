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
