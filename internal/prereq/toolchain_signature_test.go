package prereq

import "testing"

func TestValidateAuthenticodeSigner(t *testing.T) {
	tests := []struct {
		name             string
		status           string
		subject          string
		expectedIdentity string
		wantErr          bool
	}{
		{
			name:             "valid expected publisher",
			status:           "Valid",
			subject:          "CN=Epic Games Inc., O=Epic Games Inc.",
			expectedIdentity: "Epic Games Inc.",
		},
		{
			name:             "wrong publisher",
			status:           "Valid",
			subject:          "CN=Other Publisher, O=Other Publisher",
			expectedIdentity: "Epic Games Inc.",
			wantErr:          true,
		},
		{
			name:             "invalid signature status",
			status:           "HashMismatch",
			subject:          "CN=Epic Games Inc., O=Epic Games Inc.",
			expectedIdentity: "Epic Games Inc.",
			wantErr:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthenticodeSigner(tt.status, tt.subject, tt.expectedIdentity)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAuthenticodeSigner() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
