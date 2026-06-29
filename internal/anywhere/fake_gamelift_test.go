package anywhere

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// fakeGameLift is a test double for gameLiftAPI. It records which operations
// were invoked and returns canned, success-shaped responses so deployer logic
// can be tested without real AWS calls. Per-method error hooks let tests drive
// failure paths.
type fakeGameLift struct {
	// call recorders
	createdLocation     bool
	createdFleet        bool
	registeredCompute   bool
	deregisteredCompute bool
	describedFleet      bool
	deletedFleet        bool
	deletedLocation     bool
	createdSession      bool
	describedSessions   bool

	// optional error injections
	createLocationErr error
	createFleetErr    error
	registerErr       error
	deleteFleetErr    error
	deleteLocationErr error

	// canned values
	fleetStatus gltypes.FleetStatus
}

func (f *fakeGameLift) CreateLocation(_ context.Context, in *gamelift.CreateLocationInput, _ ...func(*gamelift.Options)) (*gamelift.CreateLocationOutput, error) {
	f.createdLocation = true
	if f.createLocationErr != nil {
		return nil, f.createLocationErr
	}
	return &gamelift.CreateLocationOutput{
		Location: &gltypes.LocationModel{LocationArn: aws.String("arn:loc:" + aws.ToString(in.LocationName))},
	}, nil
}

func (f *fakeGameLift) CreateFleet(_ context.Context, _ *gamelift.CreateFleetInput, _ ...func(*gamelift.Options)) (*gamelift.CreateFleetOutput, error) {
	f.createdFleet = true
	if f.createFleetErr != nil {
		return nil, f.createFleetErr
	}
	return &gamelift.CreateFleetOutput{
		FleetAttributes: &gltypes.FleetAttributes{
			FleetId:  aws.String("fleet-test"),
			FleetArn: aws.String("arn:fleet:test"),
		},
	}, nil
}

func (f *fakeGameLift) RegisterCompute(_ context.Context, _ *gamelift.RegisterComputeInput, _ ...func(*gamelift.Options)) (*gamelift.RegisterComputeOutput, error) {
	f.registeredCompute = true
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	return &gamelift.RegisterComputeOutput{
		Compute: &gltypes.Compute{GameLiftServiceSdkEndpoint: aws.String("wss://test")},
	}, nil
}

func (f *fakeGameLift) DeregisterCompute(_ context.Context, _ *gamelift.DeregisterComputeInput, _ ...func(*gamelift.Options)) (*gamelift.DeregisterComputeOutput, error) {
	f.deregisteredCompute = true
	return &gamelift.DeregisterComputeOutput{}, nil
}

func (f *fakeGameLift) DescribeFleetAttributes(_ context.Context, _ *gamelift.DescribeFleetAttributesInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeFleetAttributesOutput, error) {
	f.describedFleet = true
	status := f.fleetStatus
	if status == "" {
		status = gltypes.FleetStatusActive
	}
	return &gamelift.DescribeFleetAttributesOutput{
		FleetAttributes: []gltypes.FleetAttributes{{Status: status}},
	}, nil
}

func (f *fakeGameLift) DeleteFleet(_ context.Context, _ *gamelift.DeleteFleetInput, _ ...func(*gamelift.Options)) (*gamelift.DeleteFleetOutput, error) {
	f.deletedFleet = true
	return &gamelift.DeleteFleetOutput{}, f.deleteFleetErr
}

func (f *fakeGameLift) DeleteLocation(_ context.Context, _ *gamelift.DeleteLocationInput, _ ...func(*gamelift.Options)) (*gamelift.DeleteLocationOutput, error) {
	f.deletedLocation = true
	return &gamelift.DeleteLocationOutput{}, f.deleteLocationErr
}

func (f *fakeGameLift) CreateGameSession(_ context.Context, _ *gamelift.CreateGameSessionInput, _ ...func(*gamelift.Options)) (*gamelift.CreateGameSessionOutput, error) {
	f.createdSession = true
	return &gamelift.CreateGameSessionOutput{
		GameSession: &gltypes.GameSession{
			GameSessionId: aws.String("sess-test"),
			IpAddress:     aws.String("203.0.113.10"),
			Port:          aws.Int32(7777),
		},
	}, nil
}

func (f *fakeGameLift) DescribeGameSessions(_ context.Context, _ *gamelift.DescribeGameSessionsInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeGameSessionsOutput, error) {
	f.describedSessions = true
	return &gamelift.DescribeGameSessionsOutput{
		GameSessions: []gltypes.GameSession{{Status: gltypes.GameSessionStatusActive}},
	}, nil
}
