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
