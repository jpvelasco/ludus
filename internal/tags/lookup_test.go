package tags

import "testing"

func TestLookupEffectiveSpelling(t *testing.T) {
	values := map[string]string{"ManagedBy": "team", "managedby": "other", "project": "game"}
	for _, tt := range []struct{ key, spelling, value string }{
		{"managedby", "ManagedBy", "team"},
		{"Project", "project", "game"},
		{"missing", "missing", ""},
	} {
		t.Run(tt.key, func(t *testing.T) {
			key, value := Lookup(values, tt.key)
			if key != tt.spelling || value != tt.value {
				t.Fatalf("Lookup = %q, %q; want %q, %q", key, value, tt.spelling, tt.value)
			}
		})
	}
}
