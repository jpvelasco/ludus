package globals

import "testing"

func TestSplitImageReference(t *testing.T) {
	tests := []struct {
		name      string
		imageRef  string
		wantName  string
		wantTag   string
		wantError bool
	}{
		{name: "name and tag", imageRef: "ludus-engine:5.7", wantName: "ludus-engine", wantTag: "5.7"},
		{name: "registry path", imageRef: "registry.example.com/team/ludus-engine:5.7", wantName: "registry.example.com/team/ludus-engine", wantTag: "5.7"},
		{name: "registry port", imageRef: "localhost:5000/ludus-engine:dev", wantName: "localhost:5000/ludus-engine", wantTag: "dev"},
		{name: "untagged", imageRef: "ludus-engine", wantName: "ludus-engine", wantTag: "latest"},
		{name: "digest", imageRef: "ludus-engine@sha256:abc123", wantError: true},
		{name: "empty", imageRef: "", wantError: true},
		{name: "empty tag", imageRef: "ludus-engine:", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotTag, err := splitImageReference(tt.imageRef)
			if (err != nil) != tt.wantError {
				t.Fatalf("splitImageReference() error = %v, wantError %v", err, tt.wantError)
			}
			if gotName != tt.wantName {
				t.Errorf("splitImageReference() name = %q, want %q", gotName, tt.wantName)
			}
			if gotTag != tt.wantTag {
				t.Errorf("splitImageReference() tag = %q, want %q", gotTag, tt.wantTag)
			}
		})
	}
}
