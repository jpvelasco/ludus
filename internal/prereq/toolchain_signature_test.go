package prereq

import "testing"

type signerValidationCase struct {
	name             string
	status           string
	subject          string
	expectedIdentity string
	wantErr          bool
}

func TestValidateAuthenticodeSigner(t *testing.T) {
	runSignerValidationCases(t, []signerValidationCase{
		{
			name:             "valid publisher with other fields",
			status:           "Valid",
			subject:          "OU=Epic Games, malformed field, CN=Epic Games Inc., O=Epic Games Inc.",
			expectedIdentity: "Epic Games Inc.",
		},
		{
			name:             "wrong common name",
			status:           "Valid",
			subject:          "CN=Other Publisher, O=Epic Games Inc.",
			expectedIdentity: "Epic Games Inc.",
			wantErr:          true,
		},
		{
			name:             "wrong organization",
			status:           "Valid",
			subject:          "CN=Epic Games Inc., O=Other Publisher",
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
	})
}

func TestValidateAuthenticodeSignerRejectsMalformedSubjects(t *testing.T) {
	runSignerValidationCases(t, []signerValidationCase{
		{
			name:             "missing common name",
			status:           "Valid",
			subject:          "O=Epic Games Inc.",
			expectedIdentity: "Epic Games Inc.",
			wantErr:          true,
		},
		{
			name:             "duplicate common name",
			status:           "Valid",
			subject:          "CN=Epic Games Inc., CN=Epic Games Inc., O=Epic Games Inc.",
			expectedIdentity: "Epic Games Inc.",
			wantErr:          true,
		},
		{
			name:    "missing expected identity",
			status:  "Valid",
			subject: "CN=Epic Games Inc., O=Epic Games Inc.",
			wantErr: true,
		},
	})
}

func runSignerValidationCases(t *testing.T, tests []signerValidationCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthenticodeSigner(tt.status, tt.subject, tt.expectedIdentity)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAuthenticodeSigner() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
