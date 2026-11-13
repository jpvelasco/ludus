package deploy

import (
	"fmt"
	"strings"

	"github.com/jpvelasco/ludus/internal/tags"
)

const (
	ActionDelete        = "delete"
	ActionStop          = "stop"
	ActionDeregister    = "deregister"
	DecisionPlanned     = "planned"
	DecisionConditional = "conditional"
	DecisionSkip        = "skip"
	DecisionUnresolved  = "unresolved"
	SourceState         = "state"
	SourceConfig        = "config"
	SourceDefault       = "default"
	SourceDiscovery     = "discovery"
)

// DestroyPlan describes candidates and intent without verifying live resources.
type DestroyPlan struct {
	Version             int                 `json:"version"`
	Profile             string              `json:"profile"`
	ConfiguredRegion    string              `json:"configuredRegion,omitempty"`
	ConfiguredAccountID string              `json:"configuredAccountId,omitempty"`
	Targets             []DestroyTargetPlan `json:"targets"`
	Purge               []DestroyItem       `json:"purge"`
	Warnings            []string            `json:"warnings,omitempty"`
}

// DestroyTargetPlan preserves the execution order within one deployment target.
type DestroyTargetPlan struct {
	Target string        `json:"target"`
	Items  []DestroyItem `json:"items"`
}

// DestroyItem keeps unknown identifiers separate from discovery selectors.
type DestroyItem struct {
	Kind       string   `json:"kind"`
	Identifier string   `json:"identifier,omitempty"`
	Selector   string   `json:"selector,omitempty"`
	Source     string   `json:"source"`
	Action     string   `json:"action"`
	Decision   string   `json:"decision"`
	Conditions []string `json:"conditions,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// TargetNames is the common order for offline plans and execution sweeps.
func TargetNames() []string {
	return []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
}

// RecordedDestroyItem describes a recorded optional component. Missing
// components are skipped; it never infers an identifier from another resource.
func RecordedDestroyItem(kind, identifier, action string) DestroyItem {
	item := DestroyItem{
		Kind: kind, Identifier: identifier, Action: action,
		Source: SourceState, Decision: DecisionPlanned,
	}
	if identifier == "" {
		item.Decision = DecisionSkip
		item.Reason = "No " + kind + " recorded; execution skips this step."
	}
	return item
}

// PlanIAMRole describes the common live ownership guard without calling IAM.
func PlanIAMRole(name, source string, expected map[string]string, target, key string) (DestroyItem, error) {
	owner, err := tags.NewRoleOwnership(expected, target, key)
	if err != nil {
		return DestroyItem{}, fmt.Errorf("planning IAM role %s: %w", name, err)
	}
	return DestroyItem{
		Kind: "iam-role", Identifier: name, Source: source,
		Action: ActionDelete, Decision: DecisionConditional,
		Conditions: append([]string{"Role exists and live tags can be read."}, owner.Conditions()...),
		Reason:     "Ownership and existence are not checked offline; attribution does not prove exclusive use.",
	}, nil
}

// RenderDestroyPlan is shared by terminal output and the MCP transcript.
// Identifiers remain raw; terminal masking belongs to the existing output layer.
func RenderDestroyPlan(plan *DestroyPlan) string {
	var out strings.Builder
	out.WriteString("Dry run — no AWS calls or changes made.\n")
	profile := plan.Profile
	if profile == "" {
		profile = "default"
	}
	fmt.Fprintf(&out, "Profile: %s (target types in this profile only)\n", profile)
	for _, scope := range []struct{ name, value string }{
		{"Region", plan.ConfiguredRegion}, {"Account", plan.ConfiguredAccountID},
	} {
		value := scope.value
		if value == "" {
			value = "resolved during execution"
		}
		fmt.Fprintf(&out, "%s [%s]\n", scope.name, value)
	}
	for _, target := range plan.Targets {
		fmt.Fprintf(&out, "\n%s\n", target.Target)
		renderDestroyItems(&out, target.Items)
	}
	if len(plan.Purge) > 0 {
		out.WriteString("\npurge — selected by configured name, without an ownership tag guard\n")
		renderDestroyItems(&out, plan.Purge)
	}
	for _, warning := range plan.Warnings {
		fmt.Fprintf(&out, "\nWarning: %s\n", warning)
	}
	return out.String()
}

func renderDestroyItems(out *strings.Builder, items []DestroyItem) {
	for _, item := range items {
		fmt.Fprintf(out, "  %s %s %s [%s; %s]\n",
			item.Action, strings.ReplaceAll(item.Kind, "-", " "), item.Identifier, item.Decision, item.Source)
		if item.Selector != "" {
			fmt.Fprintf(out, "    Discover: %s\n", item.Selector)
		}
		for _, condition := range item.Conditions {
			fmt.Fprintf(out, "    Condition: %s\n", condition)
		}
		if item.Reason != "" {
			fmt.Fprintf(out, "    %s\n", item.Reason)
		}
	}
}
