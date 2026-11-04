package prereq

import (
	"fmt"
	"strings"
)

func validateAuthenticodeSigner(status, subject, expectedIdentity string) error {
	if status != "Valid" {
		return fmt.Errorf("signature status is %q, want Valid", status)
	}
	if expectedIdentity == "" {
		return fmt.Errorf("expected signer identity is empty")
	}

	fields := make(map[string]string, 2)
	for _, part := range strings.Split(subject, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "CN" && name != "O" {
			continue
		}
		if _, exists := fields[name]; exists {
			return fmt.Errorf("signer subject %q has duplicate %s fields", subject, name)
		}
		fields[name] = strings.TrimSpace(value)
	}
	if fields["CN"] != expectedIdentity || fields["O"] != expectedIdentity {
		return fmt.Errorf("signer subject %q does not match expected publisher %q", subject, expectedIdentity)
	}
	return nil
}
