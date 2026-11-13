package config

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// DefaultFleetOwnershipTagKey identifies fleets created before key overrides.
const DefaultFleetOwnershipTagKey = "ludus:fleet-name"

// ResolvedOwnershipTagKey keeps the legacy key when no override is configured.
func ResolvedOwnershipTagKey(key string) string {
	if key == "" {
		return DefaultFleetOwnershipTagKey
	}
	return key
}

// ValidateOwnershipTagKey prevents the fleet tag from replacing another
// ownership dimension or using AWS's reserved namespace.
func ValidateOwnershipTagKey(key string) error {
	key = ResolvedOwnershipTagKey(key)
	if strings.TrimSpace(key) == "" || utf8.RuneCountInString(key) > 128 {
		return fmt.Errorf("gamelift.ownershipTagKey must contain 1–128 characters")
	}
	if strings.HasPrefix(strings.ToLower(key), "aws:") {
		return fmt.Errorf("gamelift.ownershipTagKey cannot use the reserved aws: prefix")
	}
	for _, reserved := range []string{"ManagedBy", "Project", "ludus:target"} {
		if strings.EqualFold(key, reserved) {
			return fmt.Errorf("gamelift.ownershipTagKey cannot replace the %s ownership tag", reserved)
		}
	}
	return nil
}
