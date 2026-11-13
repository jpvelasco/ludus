package tags

import (
	"strings"
	"testing"
)

func TestRoleOwnershipConditions(t *testing.T) {
	for _, tt := range []struct{ target, key, want string }{
		{"gamelift", "ludus:fleet-name", "must be absent"},
		{"gamelift", "Deployment", "Legacy ludus:fleet-name"},
		{"ec2", "ludus:fleet-name", "ludus:target=\"ec2\" is required"},
	} {
		expected := map[string]string{"ManagedBy": "team", "Project": "game", tt.key: "fleet"}
		owner, err := NewRoleOwnership(expected, tt.target, tt.key)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(owner.Conditions(), "\n")
		for _, want := range []string{"No CloudFormation", "ManagedBy=\"team\"", "Project=\"game\"", tt.key + "=\"fleet\"", tt.want} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q in %s", want, text)
			}
		}
	}
}
