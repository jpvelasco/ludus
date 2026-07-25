package ec2fleet

import "testing"

func TestPackagedDirName(t *testing.T) {
	tests := []struct {
		name            string
		projectName     string
		packagedDirName string
		want            string
	}{
		// The packaged content dir is the .uproject name; ProjectName may differ.
		{"explicit packaged dir wins", "Lyra", "LyraStarterGame6", "LyraStarterGame6"},
		{"fallback to project name", "Lyra", "", "Lyra"},
		{"both empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := DeployOptions{ProjectName: tt.projectName, PackagedDirName: tt.packagedDirName}
			if got := o.packagedDirName(); got != tt.want {
				t.Errorf("packagedDirName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceTags(t *testing.T) {
	tests := []struct {
		name string
		tags map[string]string
		want map[string]string
	}{
		{"no new tags",
			make(map[string]string),
			map[string]string{"ludus:fleet-name": "testing-fleet", "ludus:target": "ec2"}},
		{"overwrite fleet name",
			map[string]string{"ludus:fleet-name": "alpha-fleet"},
			map[string]string{"ludus:fleet-name": "testing-fleet", "ludus:target": "ec2"}},
		{"overwrite fleet and merge",
			map[string]string{"ludus:fleet-name": "alpha-fleet", "ludus:env-name": "linux"},
			map[string]string{"ludus:fleet-name": "testing-fleet", "ludus:target": "ec2", "ludus:env-name": "linux"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{opts: DeployOptions{FleetName: "testing-fleet", Tags: tt.tags}}
			got := d.resourceTags()

			if len(tt.want) != len(got) {
				t.Fatalf("Map length mismatch: got %d keys, want %d keys (%v)", len(got), len(tt.want), got)
			}

			for k, wantVal := range tt.want {
				if gotVal, ok := got[k]; !ok || wantVal != gotVal {
					t.Errorf("Key %v mismatch: want %s, got %s", k, wantVal, gotVal)
				}
			}
		})
	}
}
