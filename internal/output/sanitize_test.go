package output

import "testing"

func TestMaskAccountIDs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"preview account scope",
			"Account [123456789012]\n",
			"Account [************]\n"},
		{"account derived build bucket",
			"delete s3 bucket ludus-builds-123456789012 [planned]",
			"delete s3 bucket ludus-builds-************ [planned]"},
		{"ECR URI",
			"Pushed engine image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/ludus-server:latest",
			"Pushed engine image: ************.dkr.ecr.us-east-1.amazonaws.com/ludus-server:latest"},
		{"ARN",
			"Container group definition ready: arn:aws:gamelift:us-east-1:123456789012:containergroupdefinition/ludus",
			"Container group definition ready: arn:aws:gamelift:us-east-1:************:containergroupdefinition/ludus"},
		{"bare 12-digit number untouched",
			"build id 123456789012 completed",
			"build id 123456789012 completed"},
		{"epoch timestamp untouched",
			"started at 1718000000000 ms",
			"started at 1718000000000 ms"},
		{"multiple ids on one line",
			"123456789012.dkr.ecr.us-east-1.amazonaws.com and arn:aws:s3:us-east-1:987654321098:bucket/x",
			"************.dkr.ecr.us-east-1.amazonaws.com and arn:aws:s3:us-east-1:************:bucket/x"},
		{"shorter id untouched",
			"12345678901.dkr.ecr.us-east-1.amazonaws.com",
			"12345678901.dkr.ecr.us-east-1.amazonaws.com"},
		{"empty string", "", ""},
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
