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

func assertTagJSONProperties(t *testing.T, got string, tags map[string]string) {
	t.Helper()
	for k, v := range tags {
		if !strings.Contains(got, `"Key": "`+k+`"`) {
			t.Errorf("missing key %q in output: %s", k, got)
		}
		if !strings.Contains(got, `"Value": "`+v+`"`) {
			t.Errorf("missing value %q in output: %s", v, got)
		}
	}
	trimmed := strings.TrimSpace(got)
	if !strings.HasPrefix(trimmed, "[") {
		t.Errorf("expected JSON array to start with [, got: %s", got)
	}
	if !strings.HasSuffix(trimmed, "]") {
		t.Errorf("expected JSON array to end with ], got: %s", got)
	}
	if len(tags) > 1 {
		commaCount := strings.Count(got, "},")
		expectedCommas := len(tags) - 1
		if commaCount != expectedCommas {
			t.Errorf("expected %d separators between entries, got %d", expectedCommas, commaCount)
		}
	}
}

func TestTagsToResourceJSON(t *testing.T) {
	tests := []struct {
		name string
		tags map[string]string
		want string
	}{
		{
			name: "nil map returns empty array",
			tags: nil,
			want: "[]",
		},
		{
			name: "empty map returns empty array",
			tags: map[string]string{},
			want: "[]",
		},
		{
			name: "single tag",
			tags: map[string]string{"ManagedBy": "ludus"},
		},
		{
			name: "multiple tags sorted by key",
			tags: map[string]string{
				"Zebra":  "last",
				"Alpha":  "first",
				"Middle": "mid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsToResourceJSON(tt.tags)
			if tt.want != "" {
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
				return
			}
			assertTagJSONProperties(t, got, tt.tags)
		})
	}
}

func TestTagsToResourceJSON_SortOrder(t *testing.T) {
	tags := map[string]string{
		"Charlie": "c",
		"Alpha":   "a",
		"Bravo":   "b",
	}

	got := tagsToResourceJSON(tags)

	alphaIdx := strings.Index(got, "Alpha")
	bravoIdx := strings.Index(got, "Bravo")
	charlieIdx := strings.Index(got, "Charlie")

	if alphaIdx >= bravoIdx || bravoIdx >= charlieIdx {
		t.Errorf("tags not sorted alphabetically: Alpha@%d, Bravo@%d, Charlie@%d", alphaIdx, bravoIdx, charlieIdx)
	}
}

func TestSortedKeys(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]string
		want []string
	}{
		{
			name: "nil map",
			m:    nil,
			want: []string{},
		},
		{
			name: "empty map",
			m:    map[string]string{},
			want: []string{},
		},
		{
			name: "single key",
			m:    map[string]string{"only": "one"},
			want: []string{"only"},
		},
		{
			name: "already sorted",
			m:    map[string]string{"a": "1", "b": "2", "c": "3"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "reverse order",
			m:    map[string]string{"z": "1", "m": "2", "a": "3"},
			want: []string{"a", "m", "z"},
		},
		{
			name: "mixed case sorts uppercase first",
			m:    map[string]string{"banana": "1", "Apple": "2", "cherry": "3"},
			want: []string{"Apple", "banana", "cherry"},
		},
		{
			name: "numeric string keys",
			m:    map[string]string{"10": "a", "2": "b", "1": "c"},
			want: []string{"1", "10", "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortedKeys(tt.m)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d keys, want %d", len(got), len(tt.want))
			}

			for i, key := range got {
				if key != tt.want[i] {
					t.Errorf("key[%d] = %q, want %q", i, key, tt.want[i])
				}
			}
		})
	}
}
