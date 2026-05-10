package stack

import (
	"strings"
	"testing"
)

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
