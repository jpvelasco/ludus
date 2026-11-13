package tags

import "testing"

func TestFleetOwnershipValue(t *testing.T) {
	for _, tt := range []struct {
		name    string
		tags    map[string]string
		want    string
		present bool
	}{
		{"custom", map[string]string{"Deployment": "fleet"}, "fleet", true},
		{"legacy", map[string]string{"ludus:fleet-name": "fleet"}, "fleet", true},
		{"both agree", map[string]string{"Deployment": "fleet", "ludus:fleet-name": "fleet"}, "fleet", true},
		{"both conflict", map[string]string{"Deployment": "other", "ludus:fleet-name": "fleet"}, "", false},
		{"custom conflict", map[string]string{"Deployment": "fleet", "deployment": "other"}, "", false},
		{"legacy conflict", map[string]string{"Deployment": "fleet", "ludus:fleet-name": "fleet", "Ludus:Fleet-Name": "other"}, "", false},
		{"empty custom prevents fallback", map[string]string{"Deployment": "", "ludus:fleet-name": "fleet"}, "", false},
		{"missing", nil, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, present := FleetOwnershipValue(tt.tags, "Deployment")
			if got != tt.want || present != tt.present {
				t.Fatalf("value = %q, %v; want %q, %v", got, present, tt.want, tt.present)
			}
		})
	}
}

func TestRoleOwnershipCustomFleetKey(t *testing.T) {
	expected := map[string]string{"ManagedBy": "team", "Project": "game", "Deployment": "fleet"}
	owner, err := NewRoleOwnership(expected, "gamelift", "Deployment")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Deployment", "ludus:fleet-name"} {
		observed := map[string]string{"ManagedBy": "team", "Project": "game", key: "fleet"}
		if matches, reason := owner.Match(observed); !matches {
			t.Fatalf("key %s refused: %s", key, reason)
		}
		observed[key] = "other"
		if matches, _ := owner.Match(observed); matches {
			t.Fatalf("foreign %s accepted", key)
		}
	}
	if _, err := NewRoleOwnership(expected, "gamelift", "Project"); err == nil {
		t.Fatal("reserved identity key accepted")
	}
}
