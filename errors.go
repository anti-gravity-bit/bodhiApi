package bodhiApi

import (
	"errors"
	"fmt"
	"net/http"
)

// HTTPError is an error that can be returned to an HTTP client.
//
// Status is used as the HTTP response status code. Message, Code, and Details
// are suitable for a JSON response body. Status is intentionally excluded from
// that JSON body because it is already represented by the HTTP response itself.
type HTTPError struct {
	Status  int            `json:"-"`
	Message string         `json:"message"`
	Code    string         `json:"code,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// Error implements the standard error interface.
//
// When Code is available, it is included to make logs more useful. Otherwise,
// the message is formatted with only the status code.
func (e *HTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message)
	}

	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

// NewHTTPError creates an HTTPError with a status and message.
//
// Code and Details are left empty so callers can add them only when needed.
func NewHTTPError(status int, message string) *HTTPError {
	return &HTTPError{
		Status:  status,
		Message: message,
	}
}

// NotFound creates a 404 error with the standard not_found code.
//
// An empty message is replaced with "Not Found" so the resulting error is
// always useful to an HTTP client.
func NotFound(message string) *HTTPError {
	if message == "" {
		message = "Not Found"
	}

	return &HTTPError{
		Status:  http.StatusNotFound,
		Message: message,
		Code:    "not_found",
	}
}

// MethodNotAllowed creates a 405 error.
//
// The allow value is stored in Details so callers can use it to populate an
// Allow response header or include it in a JSON error response.
func MethodNotAllowed(allow string) *HTTPError {
	return &HTTPError{
		Status:  http.StatusMethodNotAllowed,
		Message: "Method Not Allowed",
		Code:    "method_not_allowed",
		Details: map[string]any{"allow": allow},
	}
}

// AsHTTPError converts any error into an HTTPError.
//
// The conversion follows three simple rules:
//  1. nil stays nil, because there is no error to convert.
//  2. An HTTPError is returned unchanged, including when it is wrapped.
//  3. Any other error becomes a generic 500 internal error so callers always
//     have a safe HTTP response to return.
func AsHTTPError(err error) *HTTPError {
	if err == nil {
		return nil
	}

	if httpError, ok := errors.AsType[*HTTPError](err); ok {
		return httpError
	}

	return internalServerError()
}

// internalServerError creates the safe public error used when the original
// error is not an HTTPError. The original error is intentionally not exposed
// to the client.
func internalServerError() *HTTPError {
	return &HTTPError{
		Status:  http.StatusInternalServerError,
		Message: "Internal Server Error",
		Code:    "internal",
	}
}
