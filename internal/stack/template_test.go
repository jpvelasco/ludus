package stack

import (
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
