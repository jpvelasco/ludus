package prereq

import "fmt"

const memoryRequiredGB = 16

// memoryResult builds a CheckResult from a total-memory GB value.
func memoryResult(totalGB uint64) CheckResult {
	if totalGB < memoryRequiredGB {
		return CheckResult{
			Name:    "Memory",
			Passed:  false,
			Message: fmt.Sprintf("%d GB total, need %d GB", totalGB, memoryRequiredGB),
		}
	}
	return CheckResult{
		Name:    "Memory",
		Passed:  true,
		Message: fmt.Sprintf("%d GB total", totalGB),
	}
}
