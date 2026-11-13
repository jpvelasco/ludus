package ec2fleet

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestCreateGameSessionPassesFleetAndCapacity(t *testing.T) {
	client := &fakeGLClient{}
	d := &Deployer{glClient: client}
	info, err := d.CreateGameSession(context.Background(), "recorded-fleet", 12)
	if err != nil {
		t.Fatal(err)
	}
	for name, ok := range map[string]bool{
		"fleet":                aws.ToString(client.sessionInput.FleetId) == "recorded-fleet",
		"capacity":             aws.ToInt32(client.sessionInput.MaximumPlayerSessionCount) == 12,
		"no Anywhere location": client.sessionInput.Location == nil,
		"result":               info.SessionID == "session-created",
	} {
		if !ok {
			t.Errorf("%s was not preserved by the session adapter", name)
		}
	}
}

func TestDescribeGameSessionPassesRecordedID(t *testing.T) {
	client := &fakeGLClient{}
	d := &Deployer{glClient: client}
	status, err := d.DescribeGameSession(context.Background(), "recorded-session")
	if err != nil || status != "ACTIVE" {
		t.Fatalf("session status = %q, %v", status, err)
	}
	if aws.ToString(client.sessionLookupInput.GameSessionId) != "recorded-session" {
		t.Fatal("session adapter changed the requested ID")
	}
}
