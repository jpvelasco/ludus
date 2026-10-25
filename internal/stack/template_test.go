package stack

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateTemplate_NoInstanceInboundPermissions(t *testing.T) {
	tmpl := GenerateTemplate(TemplateOptions{
		ContainerGroupName: "test-group",
		ServerPort:         7777,
		ServerSDKVersion:   "5.4.0",
		Tags:               map[string]string{"ManagedBy": "ludus"},
	})

	if strings.Contains(tmpl, "InstanceInboundPermissions") {
		t.Error("template should not contain InstanceInboundPermissions — GameLift auto-calculates the port range")
	}

	if strings.Contains(tmpl, "InstanceConnectionPortRange") {
		t.Error("template should not contain InstanceConnectionPortRange — GameLift auto-calculates the port range")
	}
}

func TestGenerateTemplate_ContainsRequiredResources(t *testing.T) {
	tmpl := GenerateTemplate(TemplateOptions{
		ContainerGroupName: "test-group",
		ServerPort:         7777,
		ServerSDKVersion:   "5.4.0",
	})

	required := []string{
		"AWS::IAM::Role",
		"AWS::GameLift::ContainerGroupDefinition",
		"AWS::GameLift::ContainerFleet",
		"test-group",
		"5.4.0",
	}

	for _, r := range required {
		if !strings.Contains(tmpl, r) {
			t.Errorf("template missing required content: %q", r)
		}
	}
}

var generateTemplateTests = []struct {
	name          string
	opts          TemplateOptions
	wantContains  []string
	wantAbsent    []string
	wantValidJSON bool
}{
	{
		name: "basic options with custom port",
		opts: TemplateOptions{
			ContainerGroupName: "my-game-server",
			ServerPort:         9000,
			ServerSDKVersion:   "5.5.0",
			Tags:               map[string]string{"env": "prod"},
		},
		wantContains: []string{
			`"Default": 9000`,
			`"my-game-server"`,
			`"5.5.0"`,
			`"env"`,
			`"prod"`,
		},
		wantValidJSON: true,
	},
	{
		name: "default SDK version when empty",
		opts: TemplateOptions{
			ContainerGroupName: "default-sdk",
			ServerPort:         7777,
			ServerSDKVersion:   "",
		},
		wantContains: []string{
			`"5.4.0"`,
			`"default-sdk"`,
		},
		wantValidJSON: true,
	},
	{
		name: "no tags produces empty arrays",
		opts: TemplateOptions{
			ContainerGroupName: "no-tags-group",
			ServerPort:         7777,
			ServerSDKVersion:   "5.4.0",
			Tags:               nil,
		},
		wantContains: []string{
			"[]",
			`"no-tags-group"`,
		},
		wantValidJSON: true,
	},
	{
		name: "empty tags map produces empty arrays",
		opts: TemplateOptions{
			ContainerGroupName: "empty-tags",
			ServerPort:         7777,
			ServerSDKVersion:   "5.4.0",
			Tags:               map[string]string{},
		},
		wantContains: []string{
			"[]",
		},
		wantValidJSON: true,
	},
	{
		name: "multiple tags appear in template",
		opts: TemplateOptions{
			ContainerGroupName: "tagged-group",
			ServerPort:         7777,
			ServerSDKVersion:   "5.4.0",
			Tags: map[string]string{
				"ManagedBy":   "ludus",
				"Environment": "staging",
				"Team":        "platform",
			},
		},
		wantContains: []string{
			`"ManagedBy"`,
			`"ludus"`,
			`"Environment"`,
			`"staging"`,
			`"Team"`,
			`"platform"`,
		},
		wantValidJSON: true,
	},
	{
		name: "template contains parameters section",
		opts: TemplateOptions{
			ContainerGroupName: "param-check",
			ServerPort:         8080,
			ServerSDKVersion:   "5.4.0",
		},
		wantContains: []string{
			`"Parameters"`,
			`"ImageURI"`,
			`"ServerPort"`,
			`"InstanceType"`,
			`"Default": 8080`,
			`"c6i.large"`,
		},
		wantValidJSON: true,
	},
	{
		name: "template contains outputs section",
		opts: TemplateOptions{
			ContainerGroupName: "output-check",
			ServerPort:         7777,
			ServerSDKVersion:   "5.4.0",
		},
		wantContains: []string{
			`"Outputs"`,
			`"FleetId"`,
			`"FleetArn"`,
			`"ContainerGroupDefinitionArn"`,
			`"RoleArn"`,
		},
		wantValidJSON: true,
	},
	{
		name: "template has IAM role with gamelift principal",
		opts: TemplateOptions{
			ContainerGroupName: "iam-check",
			ServerPort:         7777,
			ServerSDKVersion:   "5.4.0",
		},
		wantContains: []string{
			`"GameLiftRole"`,
			`"gamelift.amazonaws.com"`,
			`"sts:AssumeRole"`,
			"GameLiftContainerFleetPolicy",
		},
		wantValidJSON: true,
	},
	{
		name: "container group uses UDP protocol",
		opts: TemplateOptions{
			ContainerGroupName: "proto-check",
			ServerPort:         7777,
			ServerSDKVersion:   "5.4.0",
		},
		wantContains: []string{
			`"Protocol": "UDP"`,
			`"AMAZON_LINUX_2023"`,
			`"game-server"`,
		},
		wantAbsent: []string{
			"InstanceInboundPermissions",
			"InstanceConnectionPortRange",
		},
		wantValidJSON: true,
	},
}

func TestGenerateTemplate(t *testing.T) {
	for _, tt := range generateTemplateTests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := GenerateTemplate(tt.opts)

			for _, want := range tt.wantContains {
				if !strings.Contains(tmpl, want) {
					t.Errorf("template missing expected content: %q", want)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(tmpl, absent) {
					t.Errorf("template should not contain: %q", absent)
				}
			}

			if tt.wantValidJSON {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(tmpl), &parsed); err != nil {
					t.Errorf("template is not valid JSON: %v", err)
				}
			}
		})
	}
}

func TestServerSDKVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{"empty string", "", "5.4.0"},
		{"non empty string", "5.3.2", "5.3.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := serverSDKVersion(tt.version)
			if version != tt.expected {
				t.Errorf("have %s, want %s", version, tt.expected)
			}
		})
	}
}

// TestGenerateTemplate_ServerSDKVersionOverride pins the #555 contract: a
// non-default ec2fleet.serverSdkVersion must reach the generated CloudFormation
// template instead of being replaced by the hardcoded default.
func TestGenerateTemplate_ServerSDKVersionOverride(t *testing.T) {
	tmpl := GenerateTemplate(TemplateOptions{
		ContainerGroupName: "test-group",
		ServerPort:         7777,
		ServerSDKVersion:   "5.6.0",
	})
	if !strings.Contains(tmpl, `"5.6.0"`) {
		t.Errorf("template does not carry the configured Server SDK version 5.6.0:\n%s", tmpl)
	}
	if strings.Contains(tmpl, "5.4.0") {
		t.Error("template still contains the default SDK version alongside the override")
	}
}

// assertValidTemplateJSON fails t when the generated template is not JSON.
func assertValidTemplateJSON(t *testing.T, tmpl string) {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(tmpl), &parsed); err != nil {
		t.Errorf("template is not valid JSON: %v", err)
	}
}

// TestGenerateTemplate_GameServerContainerGroupsPerInstance pins #743: the
// resolved gamelift.maxConcurrentSessions must reach the ContainerFleet's
// GameServerContainerGroupsPerInstance property, with an unset value
// resolving to the single-group default.
func TestGenerateTemplate_GameServerContainerGroupsPerInstance(t *testing.T) {
	tests := []struct {
		name string
		mcs  int
		want string
	}{
		{name: "configured value", mcs: 3, want: `"GameServerContainerGroupsPerInstance": 3`},
		{name: "unset resolves to one group per instance", mcs: 0, want: `"GameServerContainerGroupsPerInstance": 1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := GenerateTemplate(TemplateOptions{
				ContainerGroupName:    "test-group",
				ServerPort:            7777,
				ServerSDKVersion:      "5.4.0",
				MaxConcurrentSessions: tt.mcs,
			})
			if !strings.Contains(tmpl, tt.want) {
				t.Errorf("template missing expected content %q:\n%s", tt.want, tmpl)
			}
			assertValidTemplateJSON(t, tmpl)
		})
	}
}

// TestGenerateTemplate_NoPinnedRoleName pins #721: the template must not pin a
// physical IAM role name — two independent Ludus stacks in the same account
// would collide on it. CloudFormation generates a unique physical name per
// stack, and every reference resolves through the logical GameLiftRole id.
func TestGenerateTemplate_NoPinnedRoleName(t *testing.T) {
	stacks := map[string]string{
		"stack-a": "group-a",
		"stack-b": "group-b",
	}
	for name, group := range stacks {
		t.Run(name, func(t *testing.T) {
			tmpl := GenerateTemplate(TemplateOptions{
				ContainerGroupName: group,
				ServerPort:         7777,
				ServerSDKVersion:   "5.4.0",
			})
			if strings.Contains(tmpl, "RoleName") {
				t.Errorf("template pins a physical RoleName; a second stack in the same account would collide:\n%s", tmpl)
			}
			if !strings.Contains(tmpl, `"Fn::GetAtt": ["GameLiftRole", "Arn"]`) {
				t.Errorf("template must keep resolving the role via the logical GameLiftRole ref:\n%s", tmpl)
			}
			assertValidTemplateJSON(t, tmpl)
		})
	}
}
