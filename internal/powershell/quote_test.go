package powershell

import "testing"

func TestQuoteSingleQuoted(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "''"},
		{
			name:  "apostrophe and spaces",
			value: `C:\Users\a'b\Program Files`,
			want:  `'C:\Users\a''b\Program Files'`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuoteSingleQuoted(tt.value); got != tt.want {
				t.Errorf("QuoteSingleQuoted(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
