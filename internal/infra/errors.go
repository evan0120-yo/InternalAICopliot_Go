package infra

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BusinessError is the unified application error surface.
type BusinessError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *BusinessError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError creates a business-level error with a stable response shape.
func NewError(code, message string, status int) error {
	return &BusinessError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
	}
}

// IsContextCancelled normalizes standard context and gRPC cancellation errors.
func IsContextCancelled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	code := status.Code(err)
	return code == codes.Canceled || code == codes.DeadlineExceeded
}

// AsBusinessError normalizes any error into BusinessError.
func AsBusinessError(err error) *BusinessError {
	if err == nil {
		return nil
	}

	var businessErr *BusinessError
	if errors.As(err, &businessErr) {
		return businessErr
	}

	log.Printf("internal error: %v", err)

	return &BusinessError{
		Code:       "INTERNAL_SERVER_ERROR",
		Message:    "An internal error occurred.",
		HTTPStatus: http.StatusInternalServerError,
	}
}
