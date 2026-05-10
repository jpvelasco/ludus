package prereq

import (
	"strings"
	"testing"
)

func TestValidate_AllPass(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Passed: true, Message: "ok"},
		{Name: "B", Passed: true, Message: "ok"},
	}
	if err := Validate(results); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidate_WithFailure(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Passed: true, Message: "ok"},
		{Name: "B", Passed: false, Message: "missing"},
	}
	err := Validate(results)
	if err == nil {
		t.Fatal("expected error for failed check")
	}
	if !strings.Contains(err.Error(), "1 prerequisite check(s) failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidate_WarningsPass(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Passed: true, Warning: true, Message: "heads up"},
	}
	if err := Validate(results); err != nil {
		t.Errorf("warnings should not cause failure, got: %v", err)
	}
}

func TestValidate_Empty(t *testing.T) {
	if err := Validate(nil); err != nil {
		t.Errorf("empty results should pass, got: %v", err)
	}
}
