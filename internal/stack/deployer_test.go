package stack

import (
	"testing"
)

func TestStackResourceTags(t *testing.T) {
	tests := []struct {
		name         string
		deployer     *StackDeployer
		expectedTags map[string]string
	}{
		{
			name: "only fleet name provied, no custom tags",
			deployer: &StackDeployer{
				opts: StackOptions{
					FleetName: "alpha-fleet",
					Tags:      nil,
				},
			},
			expectedTags: map[string]string{
				"ludus:fleet-name": "alpha-fleet",
			},
		},
		{
			name: "merge custom tags with fleet name",
			deployer: &StackDeployer{
				opts: StackOptions{
					FleetName: "beta-fleet",
					Tags: map[string]string{
						"Environment": "Production",
						"Owner":       "GameOps",
					},
				},
			},
			expectedTags: map[string]string{
				"ludus:fleet-name": "beta-fleet",
				"Environment":      "Production",
				"Owner":            "GameOps",
			},
		},
		{
			name: "fleet name in customer tags overwritten by options field",
			deployer: &StackDeployer{
				opts: StackOptions{
					FleetName: "override-fleet",
					Tags: map[string]string{
						"ludus:fleet-name": "old-fleet",
						"Environment":      "Staging",
					},
				},
			},
			expectedTags: map[string]string{
				"ludus:fleet-name": "override-fleet",
				"Environment":      "Staging",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.deployer.stackResourceTags()
			if len(result) != len(tt.expectedTags) {
				t.Fatalf("got %d tags, want %d (result %v)", len(result), len(tt.expectedTags), result)
			}

			for k, expectedVal := range tt.expectedTags {
				val, exists := result[k]
				if !exists {
					t.Errorf("expected tag key %q was missing", val)
					continue
				}
				if val != expectedVal {
					t.Errorf("tag %q = %q, want %q", k, val, expectedVal)
				}
			}
		})
	}
}
