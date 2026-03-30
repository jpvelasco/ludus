package awsutil

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound checks if an error indicates a resource was not found.
func IsNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFoundException", "ResourceNotFoundException", "NoSuchEntity",
			"RepositoryNotFoundException", "NoSuchBucket", "NotFound":
			return true
		}
	}

	// Check for HTTP 404 status code (S3 HeadBucket returns this).
	var oe *smithy.OperationError
	if errors.As(err, &oe) {
		errStr := oe.Error()
		if errStr == "" && oe.Unwrap() != nil {
			errStr = oe.Unwrap().Error()
		}
		if errStr != "" && (errStr == "NotFound" || errStr == "404") {
			return true
		}
	}

	return false
}

// IsConflict returns true if the AWS API error code indicates a resource already exists.
func IsConflict(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ConflictException", "EntityAlreadyExistsException":
			return true
		}
	}
	return false
}
