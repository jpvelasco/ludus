package pricing

import "fmt"

// Static us-east-1 on-demand pricing (USD/hr). Covers common GameLift instance
// types. Values are approximate — actual cost varies by region.
var prices = map[string]float64{
	// Compute-optimized (most common for game servers)
	"c5.large":    0.085,
	"c5.xlarge":   0.170,
	"c5.2xlarge":  0.340,
	"c5.4xlarge":  0.680,
	"c6i.large":   0.085,
	"c6i.xlarge":  0.170,
	"c6i.2xlarge": 0.340,
	"c6i.4xlarge": 0.680,
	"c6g.large":   0.068,
	"c6g.xlarge":  0.136,
	"c6g.2xlarge": 0.272,
	"c7g.large":   0.072,
	"c7g.xlarge":  0.145,
	"c7g.2xlarge": 0.290,

	// General purpose
	"m5.large":   0.096,
	"m5.xlarge":  0.192,
	"m6i.large":  0.096,
	"m6i.xlarge": 0.192,
	"m6g.large":  0.077,
	"m6g.xlarge": 0.154,

	// Memory-optimized
	"r5.large":   0.126,
	"r5.xlarge":  0.252,
	"r6i.large":  0.126,
	"r6i.xlarge": 0.252,
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
