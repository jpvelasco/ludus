//go:build linux

package prereq

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (c *Checker) checkMemory() CheckResult {
	totalGB, err := readMemTotalGB()
	if err != nil {
		return CheckResult{Name: "Memory", Passed: false, Message: err.Error()}
	}
	return memoryResult(totalGB)
}

// readMemTotalGB reads /proc/meminfo and returns total physical memory in GB.
func readMemTotalGB() (uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("cannot read /proc/meminfo: %w", err)
	}
	return parseMemTotalKB(data)
}

// parseMemTotalKB parses /proc/meminfo content and returns total memory in GB.
func parseMemTotalKB(data []byte) (uint64, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, fmt.Errorf("malformed MemTotal line in /proc/meminfo")
			}
			kB, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("cannot parse MemTotal value: %w", err)
			}
			return kB / (1024 * 1024), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading /proc/meminfo: %w", err)
	}
	return 0, fmt.Errorf("could not parse /proc/meminfo: MemTotal line not found")
}
