package glsession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

func TestCreateExplainsCapacityAndPreservesCause(t *testing.T) {
	cause := &gltypes.FleetCapacityExceededException{Message: aws.String("No active and available server processes")}
	fake := &fakeSessionAPI{createErr: fmt.Errorf("SDK wrapper: %w", cause)}
	_, err := Create(context.Background(), fake, "fleet-example", "", 8)
	if !errors.Is(err, cause) {
		t.Fatalf("lost SDK error: %v", err)
	}
	for _, want := range []string{"fleet-example has no free game server process", "wait for a session to end", "gamelift.maxConcurrentSessions", "scale the fleet"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "1 of 1") {
		t.Fatalf("invented capacity count: %v", err)
	}
}

func TestCreateDoesNotMisclassifyOtherAPIErrors(t *testing.T) {
	cause := &gltypes.InvalidRequestException{Message: aws.String("bad request")}
	_, err := Create(context.Background(), &fakeSessionAPI{createErr: cause}, "fleet-example", "", 8)
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "no free game server process") {
		t.Fatalf("unexpected error: %v", err)
	}
}
