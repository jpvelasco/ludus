// Package glsession provides shared GameLift game session operations
// used by multiple deploy targets (gamelift, ec2fleet, stack, anywhere).
package glsession

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	"github.com/devrecon/ludus/internal/deploy"
)

// Create creates a game session on the given fleet.
// If location is non-empty it is included in the request (required for Anywhere fleets).
func Create(ctx context.Context, client *gamelift.Client, fleetID, location string, maxPlayers int) (*deploy.SessionInfo, error) {
	input := &gamelift.CreateGameSessionInput{
		FleetId:                   aws.String(fleetID),
		MaximumPlayerSessionCount: aws.Int32(int32(maxPlayers)),
	}
	if location != "" {
		input.Location = aws.String(location)
	}

	out, err := client.CreateGameSession(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("creating game session: %w", err)
	}

	info := &deploy.SessionInfo{
		SessionID: aws.ToString(out.GameSession.GameSessionId),
		IPAddress: aws.ToString(out.GameSession.IpAddress),
		Port:      int(aws.ToInt32(out.GameSession.Port)),
	}
	fmt.Printf("  Game session: %s\n  Connect: %s:%d\n", info.SessionID, info.IPAddress, info.Port)
	return info, nil
}

// Describe returns the current status of a game session.
func Describe(ctx context.Context, client *gamelift.Client, sessionID string) (string, error) {
	out, err := client.DescribeGameSessions(ctx, &gamelift.DescribeGameSessionsInput{
		GameSessionId: aws.String(sessionID),
	})
	if err != nil {
		return "", fmt.Errorf("describing game session: %w", err)
	}
	if len(out.GameSessions) == 0 {
		return "", fmt.Errorf("game session %s not found", sessionID)
	}
	return string(out.GameSessions[0].Status), nil
}
