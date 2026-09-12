// Package errors defines the safe JSON error contract returned by bodhiApi.
//
// Handlers should return HTTPError (or a helper such as NotFound) for expected
// failures. AsHTTPError converts unknown errors into a generic internal error
// so implementation details, including secrets in error strings, are not sent
// to API clients.
package errors

import (
	standardErrors "errors"
	"fmt"
	"net/http"
)

// HTTPError represents an error that can be safely returned to an HTTP client.
//
// Status is the HTTP status code and is omitted from JSON. Message is the
// human-readable body. Code is a stable machine-readable identifier. Details
// holds optional structured fields such as allowed methods for 405 responses.
type HTTPError struct {
	// Status is the HTTP status code written to the response. It is not serialized.
	Status int `json:"-"`
	// Message is the client-visible explanation of the failure.
	Message string `json:"message"`
	// Code is a stable identifier such as "not_found" or "internal".
	Code string `json:"code,omitempty"`
	// Details holds optional structured context included in the JSON body.
	Details map[string]any `json:"details,omitempty"`
}

// Error implements the standard error interface.
// When Code is set the string includes status, code, and message; otherwise it
// includes status and message only.
func (httpError *HTTPError) Error() string {
	if httpError.Code != "" {
		return fmt.Sprintf("%d %s: %s", httpError.Status, httpError.Code, httpError.Message)
	}
	return fmt.Sprintf("%d: %s", httpError.Status, httpError.Message)
}

// New creates an HTTPError with the provided status and message.
// Code and Details are left empty so callers can set them when needed.
func New(status int, message string) *HTTPError { return &HTTPError{Status: status, Message: message} }

// NotFound creates a standard not-found error with code "not_found".
// An empty message is replaced with "Not Found".
func NotFound(message string) *HTTPError {
	if message == "" {
		message = "Not Found"
	}
	return &HTTPError{Status: http.StatusNotFound, Message: message, Code: "not_found"}
}

// MethodNotAllowed creates a 405 error containing the allowed methods.
// allowedMethods is stored under details.allow for clients and loggers.
func MethodNotAllowed(allowedMethods string) *HTTPError {
	return &HTTPError{Status: http.StatusMethodNotAllowed, Message: "Method Not Allowed", Code: "method_not_allowed", Details: map[string]any{"allow": allowedMethods}}
}

// AsHTTPError preserves HTTPError values and hides unknown implementation errors.
//
// A nil input returns nil. If sourceError unwraps to *HTTPError, that value is
// returned unchanged so status, code, and message are preserved. Every other
// error becomes a generic 500 with code "internal" and message
// "Internal Server Error".
func AsHTTPError(sourceError error) *HTTPError {
	if sourceError == nil {
		return nil
	}
	if httpError, found := standardErrors.AsType[*HTTPError](sourceError); found {
		return httpError
	}
	return &HTTPError{Status: http.StatusInternalServerError, Message: "Internal Server Error", Code: "internal"}
}
