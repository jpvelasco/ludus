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
