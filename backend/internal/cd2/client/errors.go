package cd2client

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Error struct {
	Operation    string
	Code         codes.Code
	Message      string
	Retryable    bool
	Unauthorized bool
	Cause        error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}

	message := strings.TrimSpace(err.Message)
	switch {
	case strings.TrimSpace(err.Operation) != "" && message != "":
		return fmt.Sprintf("%s: %s", err.Operation, message)
	case strings.TrimSpace(err.Operation) != "":
		return err.Operation
	case message != "":
		return message
	case err.Cause != nil:
		return err.Cause.Error()
	default:
		return "cd2 client error"
	}
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func normalizeRPCError(operation string, err error) error {
	if err == nil {
		return nil
	}

	var wrapped *Error
	if errors.As(err, &wrapped) {
		return err
	}

	if statusValue, ok := status.FromError(err); ok {
		code := statusValue.Code()
		return &Error{
			Operation:    operation,
			Code:         code,
			Message:      strings.TrimSpace(statusValue.Message()),
			Retryable:    isRetryableCode(code),
			Unauthorized: code == codes.Unauthenticated || code == codes.PermissionDenied,
			Cause:        err,
		}
	}

	return &Error{
		Operation: operation,
		Code:      codes.Unknown,
		Message:   strings.TrimSpace(err.Error()),
		Cause:     err,
	}
}

func resultError(operation, message string) error {
	return &Error{
		Operation: operation,
		Code:      codes.Unknown,
		Message:   strings.TrimSpace(message),
	}
}

func isRetryableCode(code codes.Code) bool {
	switch code {
	case codes.Canceled, codes.DeadlineExceeded, codes.Unavailable, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}
