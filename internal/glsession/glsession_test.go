package glsession

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// buildCreateInput mirrors the input-building logic from Create without calling the API.
func buildCreateInput(fleetID, location string, maxPlayers int) *gamelift.CreateGameSessionInput {
	input := &gamelift.CreateGameSessionInput{
		FleetId:                   aws.String(fleetID),
		MaximumPlayerSessionCount: aws.Int32(int32(maxPlayers)),
	}
	if location != "" {
		input.Location = aws.String(location)
	}
	return input
}

// parseCreateOutput mirrors the response-parsing logic from Create.
func parseCreateOutput(out *gamelift.CreateGameSessionOutput) (sessionID, ipAddr string, port int) {
	return aws.ToString(out.GameSession.GameSessionId),
		aws.ToString(out.GameSession.IpAddress),
		int(aws.ToInt32(out.GameSession.Port))
}

// parseDescribeOutput mirrors the response-parsing logic from Describe.
func parseDescribeOutput(out *gamelift.DescribeGameSessionsOutput, sessionID string) (string, error) {
	if len(out.GameSessions) == 0 {
		return "", fmt.Errorf("game session %s not found", sessionID)
	}
	return string(out.GameSessions[0].Status), nil
}

func TestCreate_InputBuilding(t *testing.T) {
	tests := []struct {
		name        string
		fleetID     string
		location    string
		maxPlayers  int
		wantLocSet  bool
		wantLocVal  string
		wantPlayers int32
	}{
		{
			name:        "standard fleet without location",
			fleetID:     "fleet-standard",
			location:    "",
			maxPlayers:  8,
			wantLocSet:  false,
			wantPlayers: 8,
		},
		{
			name:        "anywhere fleet with location",
			fleetID:     "fleet-anywhere",
			location:    "custom-ludus-dev",
			maxPlayers:  4,
			wantLocSet:  true,
			wantLocVal:  "custom-ludus-dev",
			wantPlayers: 4,
		},
		{
			name:        "different location name",
			fleetID:     "fleet-test",
			location:    "custom-my-region",
			maxPlayers:  16,
			wantLocSet:  true,
			wantLocVal:  "custom-my-region",
			wantPlayers: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := buildCreateInput(tt.fleetID, tt.location, tt.maxPlayers)

			if got := aws.ToString(input.FleetId); got != tt.fleetID {
				t.Errorf("FleetId = %q, want %q", got, tt.fleetID)
			}
			if got := aws.ToInt32(input.MaximumPlayerSessionCount); got != tt.wantPlayers {
				t.Errorf("MaximumPlayerSessionCount = %d, want %d", got, tt.wantPlayers)
			}
			if tt.wantLocSet {
				if input.Location == nil {
					t.Error("expected Location to be set")
				} else if got := aws.ToString(input.Location); got != tt.wantLocVal {
					t.Errorf("Location = %q, want %q", got, tt.wantLocVal)
				}
			} else {
				if input.Location != nil {
					t.Errorf("expected Location to be nil, got %q", aws.ToString(input.Location))
				}
			}
		})
	}
}

func TestCreate_ResponseParsing(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		ip        string
		port      int32
	}{
		{"typical response", "gsess-123", "10.0.0.1", 7777},
		{"different port", "gsess-456", "192.168.1.5", 8888},
		{"zero port", "gsess-789", "172.16.0.1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &gamelift.CreateGameSessionOutput{
				GameSession: &gltypes.GameSession{
					GameSessionId: aws.String(tt.sessionID),
					IpAddress:     aws.String(tt.ip),
					Port:          aws.Int32(tt.port),
				},
			}

			gotID, gotIP, gotPort := parseCreateOutput(out)

			if gotID != tt.sessionID {
				t.Errorf("SessionID = %q, want %q", gotID, tt.sessionID)
			}
			if gotIP != tt.ip {
				t.Errorf("IPAddress = %q, want %q", gotIP, tt.ip)
			}
			if gotPort != int(tt.port) {
				t.Errorf("Port = %d, want %d", gotPort, tt.port)
			}
		})
	}
}

func TestDescribe_ParsesStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     gltypes.GameSessionStatus
		wantStatus string
	}{
		{"active session", gltypes.GameSessionStatusActive, "ACTIVE"},
		{"activating session", gltypes.GameSessionStatusActivating, "ACTIVATING"},
		{"terminated session", gltypes.GameSessionStatusTerminated, "TERMINATED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &gamelift.DescribeGameSessionsOutput{
				GameSessions: []gltypes.GameSession{
					{Status: tt.status},
				},
			}
			status, err := parseDescribeOutput(out, "gsess-test")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
		})
	}
}

func TestDescribe_EmptySessions(t *testing.T) {
	out := &gamelift.DescribeGameSessionsOutput{
		GameSessions: []gltypes.GameSession{},
	}

	_, err := parseDescribeOutput(out, "gsess-missing")
	if err == nil {
		t.Fatal("expected error for empty game sessions")
	}
}

// Compile-time check that Create and Describe are exported and accessible.
var (
	_ = Create
	_ = Describe
)
