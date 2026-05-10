package inventory

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestHasTag(t *testing.T) {
	tests := []struct {
		name  string
		tags  []s3types.Tag
		key   string
		value string
		want  bool
	}{
		{
			name: "matching tag",
			tags: []s3types.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("ludus")}},
			key:  "ManagedBy", value: "ludus", want: true,
		},
		{
			name: "wrong value",
			tags: []s3types.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("other")}},
			key:  "ManagedBy", value: "ludus", want: false,
		},
		{
			name: "wrong key",
			tags: []s3types.Tag{{Key: aws.String("Owner"), Value: aws.String("ludus")}},
			key:  "ManagedBy", value: "ludus", want: false,
		},
		{
			name: "empty tags",
			tags: nil,
			key:  "ManagedBy", value: "ludus", want: false,
		},
		{
			name: "multiple tags finds match",
			tags: []s3types.Tag{
				{Key: aws.String("Env"), Value: aws.String("prod")},
				{Key: aws.String("ManagedBy"), Value: aws.String("ludus")},
			},
			key: "ManagedBy", value: "ludus", want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTag(tt.tags, tt.key, tt.value)
			if got != tt.want {
				t.Errorf("hasTag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseARN(t *testing.T) {
	tests := []struct {
		name        string
		arn         string
		wantService string
		wantType    string
		wantName    string
	}{
		{
			name:        "gamelift_fleet",
			arn:         "arn:aws:gamelift:us-east-1:123456789012:fleet/fleet-abc",
			wantService: "gamelift",
			wantType:    "fleet",
			wantName:    "fleet-abc",
		},
		{
			name:        "iam_role",
			arn:         "arn:aws:iam::123456789012:role/LudusRole",
			wantService: "iam",
			wantType:    "role",
			wantName:    "LudusRole",
		},
		{
			name:        "cloudformation_stack",
			arn:         "arn:aws:cloudformation:us-east-1:123456789012:stack/my-stack/guid-123",
			wantService: "cloudformation",
			wantType:    "stack",
			wantName:    "my-stack",
		},
		{
			name:        "s3_bucket",
			arn:         "arn:aws:s3:::my-bucket",
			wantService: "s3",
			wantType:    "",
			wantName:    "my-bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, resourceType, resourceName := parseARN(tt.arn)
			if service != tt.wantService {
				t.Errorf("parseARN(%q) service = %q, want %q", tt.arn, service, tt.wantService)
			}
			if resourceType != tt.wantType {
				t.Errorf("parseARN(%q) type = %q, want %q", tt.arn, resourceType, tt.wantType)
			}
			if resourceName != tt.wantName {
				t.Errorf("parseARN(%q) name = %q, want %q", tt.arn, resourceName, tt.wantName)
			}
		})
	}
}

func TestHumanizeResourceType(t *testing.T) {
	tests := []struct {
		name         string
		service      string
		resourceType string
		want         string
	}{
		{
			name:         "gamelift_fleet",
			service:      "gamelift",
			resourceType: "fleet",
			want:         "GameLift Fleet",
		},
		{
			name:         "iam_role",
			service:      "iam",
			resourceType: "role",
			want:         "IAM Role",
		},
		{
			name:         "unknown_service",
			service:      "unknown",
			resourceType: "thing",
			want:         "Unknown Thing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanizeResourceType(tt.service, tt.resourceType)
			if got != tt.want {
				t.Errorf("humanizeResourceType(%q, %q) = %q, want %q", tt.service, tt.resourceType, got, tt.want)
			}
		})
	}
}
