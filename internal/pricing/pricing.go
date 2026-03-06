package pricing

import (
	"fmt"
	"strings"
)

// InstanceSpec describes an EC2 instance type with pricing and characteristics.
type InstanceSpec struct {
	Type     string
	Category string // "Compute", "General", "Memory"
	VCPUs    int
	MemoryGB int
	PriceUSD float64 // us-east-1 on-demand $/hr
	Arch     string  // "amd64" or "arm64"
	Note     string
}

// instances is the full catalog of known GameLift-compatible instance types.
// Static us-east-1 on-demand pricing (USD/hr). Values are approximate — actual
// cost varies by region.
var instances = []InstanceSpec{
	// Compute-optimized x86 — best for most game servers
	{"c5.large", "Compute", 2, 4, 0.085, "amd64", "previous gen"},
	{"c5.xlarge", "Compute", 4, 8, 0.170, "amd64", "previous gen"},
	{"c5.2xlarge", "Compute", 8, 16, 0.340, "amd64", "previous gen"},
	{"c5.4xlarge", "Compute", 16, 32, 0.680, "amd64", "previous gen"},
	{"c6i.large", "Compute", 2, 4, 0.085, "amd64", "recommended default"},
	{"c6i.xlarge", "Compute", 4, 8, 0.170, "amd64", "higher player counts"},
	{"c6i.2xlarge", "Compute", 8, 16, 0.340, "amd64", "large sessions"},
	{"c6i.4xlarge", "Compute", 16, 32, 0.680, "amd64", "very high capacity"},

	// Compute-optimized Graviton (ARM64)
	{"c6g.large", "Compute", 2, 4, 0.068, "arm64", "Graviton, 20% cheaper"},
	{"c6g.xlarge", "Compute", 4, 8, 0.136, "arm64", "Graviton"},
	{"c6g.2xlarge", "Compute", 8, 16, 0.272, "arm64", "Graviton"},
	{"c7g.large", "Compute", 2, 4, 0.072, "arm64", "latest Graviton"},
	{"c7g.xlarge", "Compute", 4, 8, 0.145, "arm64", "latest Graviton"},
	{"c7g.2xlarge", "Compute", 8, 16, 0.290, "arm64", "latest Graviton"},

	// General purpose
	{"m5.large", "General", 2, 8, 0.096, "amd64", "balanced CPU/memory"},
	{"m5.xlarge", "General", 4, 16, 0.192, "amd64", "balanced"},
	{"m6i.large", "General", 2, 8, 0.096, "amd64", "balanced CPU/memory"},
	{"m6i.xlarge", "General", 4, 16, 0.192, "amd64", "balanced"},
	{"m6g.large", "General", 2, 8, 0.077, "arm64", "Graviton balanced"},
	{"m6g.xlarge", "General", 4, 16, 0.154, "arm64", "Graviton balanced"},

	// Memory-optimized
	{"r5.large", "Memory", 2, 16, 0.126, "amd64", "large game state"},
	{"r5.xlarge", "Memory", 4, 32, 0.252, "amd64", "large game state"},
	{"r6i.large", "Memory", 2, 16, 0.126, "amd64", "large game state"},
	{"r6i.xlarge", "Memory", 4, 32, 0.252, "amd64", "large game state"},
}

// prices is derived from instances for backward-compatible lookups.
var prices map[string]float64

func init() {
	prices = make(map[string]float64, len(instances))
	for _, inst := range instances {
		prices[inst.Type] = inst.PriceUSD
	}
}

// EstimateCost returns the estimated hourly cost for the given instance type.
// The second return value is false if the instance type is not in the lookup table.
func EstimateCost(instanceType string) (float64, bool) {
	p, ok := prices[instanceType]
	return p, ok
}

// FormatEstimate returns a human-readable cost estimate string, or "" if the
// instance type is not in the lookup table.
func FormatEstimate(instanceType string) string {
	p, ok := prices[instanceType]
	if !ok {
		return ""
	}
	monthly := p * 24 * 30
	return fmt.Sprintf("Estimated cost: $%.3f/hr (~$%.0f/month) for %s", p, monthly, instanceType)
}

// FormatGuidance returns a formatted instance type recommendation table.
// currentType is highlighted with an arrow. arch filters to compatible instances
// ("amd64", "arm64", or "" for all).
func FormatGuidance(currentType, arch string) string {
	var sb strings.Builder
	sb.WriteString("Instance type guidance")
	if currentType != "" {
		fmt.Fprintf(&sb, " (current: %s)", currentType)
	}
	sb.WriteString(":\n\n")

	fmt.Fprintf(&sb, "  %-15s %-9s %5s %7s %8s %9s  %s\n",
		"Type", "Category", "vCPUs", "Memory", "$/hr", "~/month", "Notes")
	fmt.Fprintf(&sb, "  %-15s %-9s %5s %7s %8s %9s  %s\n",
		strings.Repeat("─", 15), strings.Repeat("─", 9), "─────", "───────",
		"────────", "─────────", strings.Repeat("─", 24))

	curated := curatedInstances(arch)
	for _, inst := range curated {
		marker := " "
		note := inst.Note
		if inst.Type == currentType {
			marker = "→"
			if note != "" {
				note += " (current)"
			} else {
				note = "current"
			}
		}
		monthly := inst.PriceUSD * 24 * 30
		fmt.Fprintf(&sb, " %s %-15s %-9s %5d %4d GB  $%-6.3f  ~$%-6.0f  %s\n",
			marker, inst.Type, inst.Category, inst.VCPUs, inst.MemoryGB,
			inst.PriceUSD, monthly, note)
	}

	sb.WriteString("\n  Compute: best for most game servers (CPU-bound tick rates)\n")
	sb.WriteString("  General: mixed CPU/memory workloads\n")
	sb.WriteString("  Memory:  open world, many players, large game state\n")
	if arch != "arm64" {
		sb.WriteString("  ARM64 (Graviton): 20-30% cheaper, requires 'ludus game build --arch arm64'\n")
	}
	return sb.String()
}

// curatedInstances returns a representative subset for the guidance table.
func curatedInstances(arch string) []InstanceSpec {
	var types []string
	switch arch {
	case "arm64":
		types = []string{
			"c6g.large", "c6g.xlarge", "c6g.2xlarge",
			"c7g.large", "c7g.xlarge",
			"m6g.large",
		}
	default: // amd64 or unspecified — show both with Graviton alternatives
		types = []string{
			"c6i.large", "c6i.xlarge", "c6i.2xlarge",
			"c6g.large", "c7g.large",
			"m6i.large",
			"r6i.large",
		}
	}

	lookup := make(map[string]bool, len(types))
	for _, t := range types {
		lookup[t] = true
	}

	var result []InstanceSpec
	for _, inst := range instances {
		if lookup[inst.Type] {
			result = append(result, inst)
		}
	}
	return result
}

// DefaultInstanceType returns the recommended default instance type for the given architecture.
func DefaultInstanceType(arch string) string {
	if arch == "arm64" {
		return "c7g.large"
	}
	return "c6i.large"
}

// InstanceArch returns the architecture of a known instance type, or "" if unknown.
func InstanceArch(instanceType string) string {
	for _, inst := range instances {
		if inst.Type == instanceType {
			return inst.Arch
		}
	}
	return ""
}

// FormatSuggestion returns a brief one-line savings tip, or "" if none applicable.
// For amd64 compute instances, suggests the Graviton equivalent.
func FormatSuggestion(currentType, arch string) string {
	if arch == "arm64" {
		return "" // already on Graviton
	}

	// Map amd64 instances to Graviton equivalents
	gravitonAlt := map[string]string{
		"c5.large":    "c6g.large",
		"c5.xlarge":   "c6g.xlarge",
		"c5.2xlarge":  "c6g.2xlarge",
		"c6i.large":   "c6g.large",
		"c6i.xlarge":  "c6g.xlarge",
		"c6i.2xlarge": "c6g.2xlarge",
		"m5.large":    "m6g.large",
		"m5.xlarge":   "m6g.xlarge",
		"m6i.large":   "m6g.large",
		"m6i.xlarge":  "m6g.xlarge",
	}

	alt, ok := gravitonAlt[currentType]
	if !ok {
		return ""
	}

	currentPrice, cOK := prices[currentType]
	altPrice, aOK := prices[alt]
	if !cOK || !aOK {
		return ""
	}

	savings := (1 - altPrice/currentPrice) * 100
	return fmt.Sprintf("Tip: Use --arch arm64 with %s for ~%.0f%% savings ($%.3f/hr vs $%.3f/hr)",
		alt, savings, altPrice, currentPrice)
}
