package tags

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/jpvelasco/ludus/internal/config"
)

// RoleOwnership identifies roles that a direct deployer may modify.
// Tags establish cooperative attribution, not exclusive use or authorization.
type RoleOwnership struct {
	identity map[string]string
	target   string
	keys     []string
}

// NewRoleOwnership validates the effective identity written during creation.
// target is the deployer's target, never a value inferred from user tags.
func NewRoleOwnership(expected map[string]string, target, fleetKey string) (*RoleOwnership, error) {
	if err := config.ValidateOwnershipTagKey(fleetKey); err != nil {
		return nil, err
	}
	if target != "gamelift" && target != "ec2" {
		return nil, fmt.Errorf("unsupported role ownership target %q", target)
	}
	effective := Effective(expected)
	keys := []string{"ManagedBy", "Project", config.ResolvedOwnershipTagKey(fleetKey)}
	identity := make(map[string]string, len(keys))
	for _, key := range keys {
		value, _ := roleTagValue(effective, key)
		if value == "" {
			return nil, fmt.Errorf("role ownership requires a nonempty %s tag", key)
		}
		identity[key] = value
	}
	if value, present := roleTagValue(effective, "ludus:target"); present && value != target {
		return nil, fmt.Errorf("role ownership ludus:target must match deployer target %q", target)
	}
	return &RoleOwnership{identity: identity, target: target, keys: keys}, nil
}

// Match compares identity keys without case folding their values. Conflicting
// key variants and CloudFormation ownership metadata always refuse the match.
func (o *RoleOwnership) Match(observed map[string]string) (bool, string) {
	for key := range observed {
		if strings.HasPrefix(strings.ToLower(key), "aws:cloudformation:") {
			return false, "role is managed by CloudFormation"
		}
	}
	for _, key := range o.keys {
		value, present := roleTagValue(observed, key)
		if key == o.keys[2] {
			value, present = FleetOwnershipValue(observed, key)
		}
		if !present {
			return false, fmt.Sprintf("missing or ambiguous %s ownership tag", key)
		}
		if value != o.identity[key] {
			return false, fmt.Sprintf("%s ownership tag differs from this deployment", key)
		}
	}
	return o.matchTarget(observed)
}

func (o *RoleOwnership) matchTarget(observed map[string]string) (bool, string) {
	value, present := roleTagValue(observed, "ludus:target")
	if hasKeyFold(observed, "ludus:target") && !present {
		return false, "ambiguous ludus:target ownership tag"
	}
	if (present && value != o.target) || (!present && o.target == "ec2") {
		return false, "ludus:target ownership tag differs from this deployment"
	}
	return true, ""
}

// Conditions describes this guard for an offline plan without claiming that
// any observed resource satisfies it.
func (o *RoleOwnership) Conditions() []string {
	conditions := []string{"No CloudFormation ownership metadata or conflicting tag variants."}
	for _, key := range o.keys {
		condition := fmt.Sprintf("%s=%q (case-insensitive key; exact value).", key, o.identity[key])
		if key == o.keys[2] && !strings.EqualFold(key, config.DefaultFleetOwnershipTagKey) {
			condition += " Legacy ludus:fleet-name is accepted only when the configured key is absent."
		}
		conditions = append(conditions, condition)
	}
	target := fmt.Sprintf("ludus:target=%q is required.", o.target)
	if o.target == "gamelift" {
		target = "ludus:target must be absent or equal to \"gamelift\"."
	}
	return append(conditions, target)
}

// MatchIAM preserves conflicting duplicate keys when adapting IAM's tag list.
func (o *RoleOwnership) MatchIAM(observed []iamtypes.Tag) (bool, string) {
	values := make(map[string]string, len(observed))
	for _, tag := range observed {
		if tag.Key == nil || tag.Value == nil {
			return false, "malformed IAM role tags"
		}
		key, value := aws.ToString(tag.Key), aws.ToString(tag.Value)
		if previous, exists := values[key]; exists && previous != value {
			return false, "conflicting duplicate IAM role tags"
		}
		values[key] = value
	}
	return o.Match(values)
}

// roleTagValue rejects conflicting variants instead of discarding evidence.
func roleTagValue(values map[string]string, key string) (string, bool) {
	var value string
	var present bool
	for k, v := range values {
		if !strings.EqualFold(k, key) {
			continue
		}
		if present && value != v {
			return "", false
		}
		value, present = v, true
	}
	return value, present
}
