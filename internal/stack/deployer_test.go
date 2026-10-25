package stack

import (
	"testing"
)

// TestStackDeployerTemplateOptions pins the StackOptions → TemplateOptions
// wiring Deploy uses (#743): configured values are forwarded untouched and the
// resource tags carry the fleet name.
func TestStackDeployerTemplateOptions(t *testing.T) {
	tests := []struct {
		name string
		opts StackOptions
	}{
		{
			name: "configured values forwarded",
			opts: StackOptions{
				ContainerGroupName:    "cg",
				ServerPort:            7777,
				ServerSDKVersion:      "5.5.0",
				MaxConcurrentSessions: 3,
				FleetName:             "beta-fleet",
			},
		},
		{
			name: "empty server SDK version forwarded raw",
			opts: StackOptions{ContainerGroupName: "cg", ServerPort: 7777, FleetName: "beta-fleet"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&StackDeployer{opts: tt.opts}).templateOptions()
			if got.ContainerGroupName != tt.opts.ContainerGroupName {
				t.Errorf("ContainerGroupName = %q, want %q", got.ContainerGroupName, tt.opts.ContainerGroupName)
			}
			if got.ServerPort != tt.opts.ServerPort {
				t.Errorf("ServerPort = %d, want %d", got.ServerPort, tt.opts.ServerPort)
			}
			if got.ServerSDKVersion != tt.opts.ServerSDKVersion {
				t.Errorf("ServerSDKVersion = %q, want %q", got.ServerSDKVersion, tt.opts.ServerSDKVersion)
			}
			if got.MaxConcurrentSessions != tt.opts.MaxConcurrentSessions {
				t.Errorf("MaxConcurrentSessions = %d, want %d", got.MaxConcurrentSessions, tt.opts.MaxConcurrentSessions)
			}
			if got.Tags["ludus:fleet-name"] != tt.opts.FleetName {
				t.Errorf("Tags[ludus:fleet-name] = %q, want %q", got.Tags["ludus:fleet-name"], tt.opts.FleetName)
			}
		})
	}
}

func TestStackResourceTags(t *testing.T) {
	tests := []struct {
		name         string
		deployer     *StackDeployer
		expectedTags map[string]string
	}{
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
			name: "fleet name in custom tags overwritten by options field",
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
					t.Errorf("expected tag key %q was missing", k)
					continue
				}
				if val != expectedVal {
					t.Errorf("tag %q = %q, want %q", k, val, expectedVal)
				}
			}
		})
	}
}
