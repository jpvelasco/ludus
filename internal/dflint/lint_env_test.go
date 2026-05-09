package dflint

import "testing"

func TestCheckSensitiveEnv_NoSecrets(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04
ENV APP_NAME=myapp
ENV PORT=8080
`
	findings := checkSensitiveEnv(dockerfile)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheckSensitiveEnv_WithSecrets(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"password", "ENV DB_PASSWORD=secret123"},
		{"secret", "ENV API_SECRET=abc"},
		{"token", "ENV AUTH_TOKEN=xyz"},
		{"key", "ENV AWS_SECRET_KEY=foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dockerfile := "FROM ubuntu:22.04\n" + tt.line + "\n"
			findings := checkSensitiveEnv(dockerfile)
			if len(findings) != 1 {
				t.Fatalf("expected 1 finding for %s, got %d", tt.name, len(findings))
			}
			if findings[0].Level != SeverityError {
				t.Errorf("expected error severity, got %s", findings[0].Level)
			}
		})
	}
}
