package awsutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
)

// testAPIError implements smithy.APIError for testing.
type testAPIError struct {
	code    string
	message string
}

func (e *testAPIError) Error() string                 { return e.message }
func (e *testAPIError) ErrorCode() string             { return e.code }
func (e *testAPIError) ErrorMessage() string          { return e.message }
func (e *testAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"non-API error", errors.New("something else"), false},
		{"NotFoundException", &testAPIError{code: "NotFoundException"}, true},
		{"ResourceNotFoundException", &testAPIError{code: "ResourceNotFoundException"}, true},
		{"NoSuchEntity", &testAPIError{code: "NoSuchEntity"}, true},
		{"RepositoryNotFoundException", &testAPIError{code: "RepositoryNotFoundException"}, true},
		{"NoSuchBucket", &testAPIError{code: "NoSuchBucket"}, true},
		{"NotFound", &testAPIError{code: "NotFound"}, true},
		{"unrelated API error", &testAPIError{code: "ValidationException"}, false},
		{"wrapped not-found", fmt.Errorf("operation failed: %w", &testAPIError{code: "NotFoundException"}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"non-API error", errors.New("something else"), false},
		{"ConflictException", &testAPIError{code: "ConflictException"}, true},
		{"EntityAlreadyExistsException", &testAPIError{code: "EntityAlreadyExistsException"}, true},
		{"unrelated API error", &testAPIError{code: "ValidationException"}, false},
		{"wrapped conflict", fmt.Errorf("operation failed: %w", &testAPIError{code: "ConflictException"}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConflict(tt.err); got != tt.want {
				t.Errorf("IsConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}
