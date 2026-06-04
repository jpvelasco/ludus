package prereq

import (
	"strings"
	"testing"
)

func TestMemoryResult_PassAboveThreshold(t *testing.T) {
	result := memoryResult(32)
	if !result.Passed {
		t.Errorf("expected pass for 32 GB, got: %s", result.Message)
	}
	if result.Name != "Memory" {
		t.Errorf("expected name 'Memory', got: %s", result.Name)
	}
	if !strings.Contains(result.Message, "32 GB") {
		t.Errorf("expected GB count in message, got: %s", result.Message)
	}
}

func TestMemoryResult_PassAtThreshold(t *testing.T) {
	result := memoryResult(memoryRequiredGB)
	if !result.Passed {
		t.Errorf("expected pass at exactly %d GB, got: %s", memoryRequiredGB, result.Message)
	}
}

func TestMemoryResult_FailBelowThreshold(t *testing.T) {
	result := memoryResult(8)
	if result.Passed {
		t.Errorf("expected fail for 8 GB, got pass")
	}
	if !strings.Contains(result.Message, "8 GB total") {
		t.Errorf("expected current GB in message, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "need") {
		t.Errorf("expected 'need' in message, got: %s", result.Message)
	}
}

func TestMemoryResult_ZeroFails(t *testing.T) {
	result := memoryResult(0)
	if result.Passed {
		t.Errorf("expected fail for 0 GB")
	}
}
