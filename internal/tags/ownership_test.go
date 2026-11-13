package tags

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

func roleIdentity() map[string]string {
	return map[string]string{"ManagedBy": "example-team", "Project": "example-game", "ludus:fleet-name": "example-fleet"}
}

func TestRoleOwnershipInvalidIdentity(t *testing.T) {
	tests := []struct {
		name, key, value, target string
	}{
		{"empty manager", "ManagedBy", "", "gamelift"},
		{"empty project", "Project", "", "gamelift"},
		{"empty fleet", "ludus:fleet-name", "", "gamelift"},
		{"conflicting target", "ludus:target", "ec2", "gamelift"},
		{"empty explicit target", "ludus:target", "", "gamelift"},
		{"unknown target", "", "", "stack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := roleIdentity()
			expected[tt.key] = tt.value
			if _, err := NewRoleOwnership(expected, tt.target); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	if _, err := NewRoleOwnership(nil, "gamelift"); err == nil {
		t.Fatal("missing identity accepted")
	}
}

func TestRoleOwnershipMatch(t *testing.T) {
	tests := []struct {
		name, key, value, target, reason string
	}{
		{"custom manager", "", "", "gamelift", ""},
		{"unrelated tag", "Environment", "other", "gamelift", ""},
		{"matching variant", "project", "example-game", "gamelift", ""},
		{"conflicting variant", "project", "other", "gamelift", "ambiguous Project"},
		{"wrong project", "Project", "other", "gamelift", "Project"},
		{"wrong fleet", "ludus:fleet-name", "other", "gamelift", "ludus:fleet-name"},
		{"exact manager value", "ManagedBy", "Example-team", "gamelift", "ManagedBy"},
		{"empty manager", "ManagedBy", "", "gamelift", "ManagedBy"},
		{"foreign manager", "aws:cloudformation:stack-id", "example", "gamelift", "CloudFormation"},
		{"foreign manager casing", "AWS:CloudFormation:Stack-Name", "example", "gamelift", "CloudFormation"},
		{"explicit target", "ludus:target", "gamelift", "gamelift", ""},
		{"wrong target", "ludus:target", "ec2", "gamelift", "ludus:target"},
		{"ec2 missing target", "", "", "ec2", "ludus:target"},
		{"ec2 target", "ludus:target", "ec2", "ec2", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, err := NewRoleOwnership(roleIdentity(), tt.target)
			if err != nil {
				t.Fatal(err)
			}
			observed := roleIdentity()
			observed[tt.key] = tt.value
			matches, reason := owner.Match(observed)
			if matches != (tt.reason == "") || !strings.Contains(reason, tt.reason) {
				t.Fatalf("Match() = %v, %q; expected reason %q", matches, reason, tt.reason)
			}
		})
	}
}

func TestRoleOwnershipMissingAndAmbiguousTags(t *testing.T) {
	owner, err := NewRoleOwnership(roleIdentity(), "gamelift")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range roleIdentityKeys {
		t.Run(key, func(t *testing.T) {
			observed := roleIdentity()
			delete(observed, key)
			if matches, _ := owner.Match(observed); matches {
				t.Fatal("missing identity accepted")
			}
		})
	}
	observed := Merge(roleIdentity(), map[string]string{"ludus:target": "gamelift", "Ludus:Target": "ec2"})
	if matches, reason := owner.Match(observed); matches || !strings.Contains(reason, "ambiguous") {
		t.Fatalf("conflicting target accepted: %v, %q", matches, reason)
	}
}

func TestRoleOwnershipEffectiveCasing(t *testing.T) {
	expected := Merge(roleIdentity(), map[string]string{"managedby": "ignored", "project": "ignored"})
	owner, err := NewRoleOwnership(expected, "gamelift")
	if err != nil {
		t.Fatal(err)
	}
	observed := map[string]string{"MANAGEDBY": "example-team", "project": "example-game", "LUDUS:FLEET-NAME": "example-fleet"}
	if matches, reason := owner.Match(observed); !matches {
		t.Fatal(reason)
	}
	if matches, reason := owner.MatchIAM(ToIAMTags(expected)); !matches {
		t.Fatalf("creation and comparison disagree: %s", reason)
	}
}

func TestRoleOwnershipIAMTags(t *testing.T) {
	owner, err := NewRoleOwnership(roleIdentity(), "gamelift")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		tag  iamtypes.Tag
		want bool
	}{
		{"missing key", iamtypes.Tag{Value: aws.String("example-team")}, false},
		{"missing value", iamtypes.Tag{Key: aws.String("ManagedBy")}, false},
		{"conflicting exact duplicate", iamtypes.Tag{Key: aws.String("ManagedBy"), Value: aws.String("other")}, false},
		{"matching duplicate", iamtypes.Tag{Key: aws.String("ManagedBy"), Value: aws.String("example-team")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, reason := owner.MatchIAM(append(ToIAMTags(roleIdentity()), tt.tag))
			if matches != tt.want {
				t.Fatalf("MatchIAM() = %v, %q; want %v", matches, reason, tt.want)
			}
		})
	}
}
