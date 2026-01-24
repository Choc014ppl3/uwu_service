package errors

import (
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCode represents application error codes.
type ErrorCode string

const (
	// General errors
	ErrInternal     ErrorCode = "INTERNAL_ERROR"
	ErrValidation   ErrorCode = "VALIDATION_ERROR"
	ErrNotFound     ErrorCode = "NOT_FOUND"
	ErrUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrForbidden    ErrorCode = "FORBIDDEN"
	ErrConflict     ErrorCode = "CONFLICT"
	ErrRateLimit    ErrorCode = "RATE_LIMIT_EXCEEDED"

	// Service-specific errors
	ErrAIService      ErrorCode = "AI_SERVICE_ERROR"
	ErrStorageService ErrorCode = "STORAGE_SERVICE_ERROR"
	ErrPubSubService  ErrorCode = "PUBSUB_SERVICE_ERROR"
)

// AppError represents an application error with code and metadata.
type AppError struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
	Err     error                  `json:"-"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error.
func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new AppError.
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an existing error with an AppError.
func Wrap(code ErrorCode, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// WithDetails adds details to the error.
func (e *AppError) WithDetails(details map[string]interface{}) *AppError {
	e.Details = details
	return e
}

// HTTPStatus returns the HTTP status code for the error.
func (e *AppError) HTTPStatus() int {
	switch e.Code {
	case ErrValidation:
		return http.StatusBadRequest
	case ErrUnauthorized:
		return http.StatusUnauthorized
	case ErrForbidden:
		return http.StatusForbidden
	case ErrNotFound:
		return http.StatusNotFound
	case ErrConflict:
		return http.StatusConflict
	case ErrRateLimit:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// GRPCStatus returns the gRPC status for the error.
func (e *AppError) GRPCStatus() *status.Status {
	var code codes.Code
	switch e.Code {
	case ErrValidation:
		code = codes.InvalidArgument
	case ErrUnauthorized:
		code = codes.Unauthenticated
	case ErrForbidden:
		code = codes.PermissionDenied
	case ErrNotFound:
		code = codes.NotFound
	case ErrConflict:
		code = codes.AlreadyExists
	case ErrRateLimit:
		code = codes.ResourceExhausted
	default:
		code = codes.Internal
	}
	return status.New(code, e.Message)
}

// Common error constructors
func Internal(message string) *AppError {
	return New(ErrInternal, message)
}

func InternalWrap(message string, err error) *AppError {
	return Wrap(ErrInternal, message, err)
}

func Validation(message string) *AppError {
	return New(ErrValidation, message)
}

func NotFound(resource string) *AppError {
	return New(ErrNotFound, fmt.Sprintf("%s not found", resource))
}

func Unauthorized(message string) *AppError {
	return New(ErrUnauthorized, message)
}

func Forbidden(message string) *AppError {
	return New(ErrForbidden, message)
}

func RateLimit(message string) *AppError {
	return New(ErrRateLimit, message)
}
