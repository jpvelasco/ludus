package stack

import (
	"errors"
	"testing"
)

func TestIsStackNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"generic error", errors.New("something went wrong"), false},
		{"does not exist", errors.New("Stack with id my-stack does not exist"), true},
		{"NotFoundException", errors.New("ValidationError: Stack my-stack NotFoundException"), true},
		{"NotFound substring", errors.New("Stack NotFound"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStackNotFound(tt.err)
			if result != tt.expected {
				t.Errorf("isStackNotFound(%v) = %v; want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestStackDeletionPollResult(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		reason       string
		wantFinished bool
		wantErr      bool
	}{
		{"Delete Complete", "DELETE_COMPLETE", "", true, false},
		{"Delete Failure", "DELETE_FAILED", "resource blocked", false, true},
		{"Delete in Progress", "DELETE_IN_PROGRESS", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finished, err := stackDeletionPollResult(tt.status, tt.reason)
			if finished != tt.wantFinished {
				t.Errorf("finished = %v, want %v", finished, tt.wantFinished)
			}
			if (err != nil) != tt.wantErr {
t.Errorf("error status = %v, want error = %v", err, tt.wantErr)
			}
		})
	}
}
