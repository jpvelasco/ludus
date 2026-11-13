package tags

import (
	"strings"

	"github.com/jpvelasco/ludus/internal/config"
)

// FleetOwnershipValue reads the configured key, with a legacy fallback only
// when that key is absent. Conflicting key variants or legacy values refuse
// attribution instead of silently choosing one.
func FleetOwnershipValue(values map[string]string, key string) (string, bool) {
	key = config.ResolvedOwnershipTagKey(key)
	value, present := roleTagValue(values, key)
	if strings.EqualFold(key, config.DefaultFleetOwnershipTagKey) {
		return value, present
	}
	legacy, legacyPresent := roleTagValue(values, config.DefaultFleetOwnershipTagKey)
	if !hasKeyFold(values, key) {
		return legacy, legacyPresent
	}
	if hasKeyFold(values, config.DefaultFleetOwnershipTagKey) && (!legacyPresent || legacy != value) {
		return "", false
	}
	return value, present
}
