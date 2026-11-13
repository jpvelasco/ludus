package tags

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// RoleOwnership identifies roles that a direct deployer may modify.
// Tags establish cooperative attribution, not exclusive use or authorization.
type RoleOwnership struct {
	identity map[string]string
	target   string
}

var roleIdentityKeys = []string{"ManagedBy", "Project", "ludus:fleet-name"}

// NewRoleOwnership validates the effective identity written during creation.
// target is the deployer's target, never a value inferred from user tags.
func NewRoleOwnership(expected map[string]string, target string) (*RoleOwnership, error) {
	if target != "gamelift" && target != "ec2" {
		return nil, fmt.Errorf("unsupported role ownership target %q", target)
	}
	effective := dedupeFold(expected)
	identity := make(map[string]string, len(roleIdentityKeys))
	for _, key := range roleIdentityKeys {
		value, _ := roleTagValue(effective, key)
		if value == "" {
			return nil, fmt.Errorf("role ownership requires a nonempty %s tag", key)
		}
		identity[key] = value
	}
	if value, present := roleTagValue(effective, "ludus:target"); present && value != target {
		return nil, fmt.Errorf("role ownership ludus:target must match deployer target %q", target)
	}
	return &RoleOwnership{identity: identity, target: target}, nil
}

// Match compares identity keys without case folding their values. Conflicting
// key variants and CloudFormation ownership metadata always refuse the match.
func (o *RoleOwnership) Match(observed map[string]string) (bool, string) {
	for key := range observed {
		if strings.HasPrefix(strings.ToLower(key), "aws:cloudformation:") {
			return false, "role is managed by CloudFormation"
		}
	}
	for _, key := range roleIdentityKeys {
		value, present := roleTagValue(observed, key)
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
