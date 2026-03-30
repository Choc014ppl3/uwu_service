package errors

import "fmt"

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
	ErrCacheService   ErrorCode = "CACHE_SERVICE_ERROR"
	ErrDatabase       ErrorCode = "DATABASE_ERROR"
	ErrTimeout        ErrorCode = "TIMEOUT_ERROR"
)

// AppError represents an application error with code and metadata.
type AppError struct {
	code    ErrorCode
	message string
	details map[string]interface{}
	err     error
}

// Error implements the standard Go error interface.
func (e *AppError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %s: %v", e.code, e.message, e.err)
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

// Unwrap returns the wrapped error.
func (e *AppError) Unwrap() error {
	return e.err
}

// --- Getter Methods สำหรับให้ Interface ใน response.go เรียกใช้ ---

func (e *AppError) GetCode() string {
	return string(e.code)
}

// GetMessage คืนค่าเฉพาะข้อความที่ปลอดภัยสำหรับส่งให้ User (ซ่อน e.err)
func (e *AppError) GetMessage() string {
	return e.message
}

func (e *AppError) GetDetails() map[string]interface{} {
	return e.details
}

// --- Constructors ---

func New(code ErrorCode, message string) *AppError {
	return &AppError{
		code:    code,
		message: message,
	}
}

func Wrap(code ErrorCode, message string, err error) *AppError {
	return &AppError{
		code:    code,
		message: message,
		err:     err,
	}
}

func (e *AppError) WithDetails(details map[string]interface{}) *AppError {
	e.details = details
	return e
}

// --- Common Error Helpers ---

func Internal(message string) *AppError                { return New(ErrInternal, message) }
func InternalWrap(message string, err error) *AppError { return Wrap(ErrInternal, message, err) }

func Validation(message string) *AppError                { return New(ErrValidation, message) }
func ValidationWrap(message string, err error) *AppError { return Wrap(ErrValidation, message, err) }

func NotFound(message string) *AppError                { return New(ErrNotFound, message) }
func NotFoundWrap(message string, err error) *AppError { return Wrap(ErrNotFound, message, err) }

func Unauthorized(message string) *AppError { return New(ErrUnauthorized, message) }
func UnauthorizedWrap(message string, err error) *AppError {
	return Wrap(ErrUnauthorized, message, err)
}

func Forbidden(message string) *AppError                { return New(ErrForbidden, message) }
func ForbiddenWrap(message string, err error) *AppError { return Wrap(ErrForbidden, message, err) }

func Conflict(message string) *AppError                { return New(ErrConflict, message) }
func ConflictWrap(message string, err error) *AppError { return Wrap(ErrConflict, message, err) }

func RateLimit(message string) *AppError                { return New(ErrRateLimit, message) }
func RateLimitWrap(message string, err error) *AppError { return Wrap(ErrRateLimit, message, err) }
