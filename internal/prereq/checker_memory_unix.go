//go:build !windows && !linux && !darwin

package prereq

func (c *Checker) checkMemory() CheckResult {
	return CheckResult{
		Name:    "Memory",
		Passed:  true,
		Warning: true,
		Message: "memory check not available on this platform",
	}
}
