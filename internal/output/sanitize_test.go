package output

import "testing"

func TestMaskAccountIDs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ECR URI",
			in:   "Pushed engine image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/ludus-server:latest",
			want: "Pushed engine image: ************.dkr.ecr.us-east-1.amazonaws.com/ludus-server:latest",
		},
		{
			name: "ARN",
			in:   "Container group definition ready: arn:aws:gamelift:us-east-1:123456789012:containergroupdefinition/ludus",
			want: "Container group definition ready: arn:aws:gamelift:us-east-1:************:containergroupdefinition/ludus",
		},
		{
			name: "bare 12-digit number untouched",
			in:   "build id 123456789012 completed",
			want: "build id 123456789012 completed",
		},
		{
			name: "epoch timestamp untouched",
			in:   "started at 1718000000000 ms",
			want: "started at 1718000000000 ms",
		},
		{
			name: "multiple ids on one line",
			in:   "123456789012.dkr.ecr.us-east-1.amazonaws.com and arn:aws:s3:us-east-1:987654321098:bucket/x",
			want: "************.dkr.ecr.us-east-1.amazonaws.com and arn:aws:s3:us-east-1:************:bucket/x",
		},
		{
			name: "shorter id untouched",
			in:   "12345678901.dkr.ecr.us-east-1.amazonaws.com",
			want: "12345678901.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskAccountIDs(tt.in)
			if got != tt.want {
				t.Errorf("MaskAccountIDs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
