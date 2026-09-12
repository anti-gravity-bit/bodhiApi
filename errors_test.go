package bodhiApi

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
)

// TestHTTPErrorError verifies both Error string formats: with and without a
// machine-readable error code.
func TestHTTPErrorError(t *testing.T) {
	tests := []struct {
		name string
		err  *HTTPError
		want string
	}{
		{
			name: "with code",
			err:  &HTTPError{Status: http.StatusNotFound, Code: "not_found", Message: "Missing"},
			want: "404 not_found: Missing",
		},
		{
			name: "without code",
			err:  &HTTPError{Status: http.StatusBadRequest, Message: "Invalid request"},
			want: "400: Invalid request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNewHTTPError verifies that the general constructor sets only the values
// supplied by its arguments.
func TestNewHTTPError(t *testing.T) {
	got := NewHTTPError(http.StatusBadRequest, "Invalid request")
	want := &HTTPError{
		Status:  http.StatusBadRequest,
		Message: "Invalid request",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NewHTTPError() = %#v, want %#v", got, want)
	}
}

// TestNotFound verifies the standard code and the default message behavior.
func TestNotFound(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    *HTTPError
	}{
		{
			name:    "custom message",
			message: "User not found",
			want: &HTTPError{
				Status:  http.StatusNotFound,
				Message: "User not found",
				Code:    "not_found",
			},
		},
		{
			name:    "default message",
			message: "",
			want: &HTTPError{
				Status:  http.StatusNotFound,
				Message: "Not Found",
				Code:    "not_found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NotFound(tt.message); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NotFound(%q) = %#v, want %#v", tt.message, got, tt.want)
			}
		})
	}
}

// TestMethodNotAllowed verifies the standard 405 error and its allow detail.
func TestMethodNotAllowed(t *testing.T) {
	got := MethodNotAllowed("GET, POST")
	want := &HTTPError{
		Status:  http.StatusMethodNotAllowed,
		Message: "Method Not Allowed",
		Code:    "method_not_allowed",
		Details: map[string]any{"allow": "GET, POST"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MethodNotAllowed() = %#v, want %#v", got, want)
	}
}

// TestAsHTTPError verifies nil handling, direct HTTPErrors, wrapped HTTPErrors,
// and conversion of unrelated errors into the generic internal error.
func TestAsHTTPError(t *testing.T) {
	direct := NewHTTPError(http.StatusTeapot, "short and stout")

	// Use a standard wrapping error so errors.AsType can locate the original
	// HTTPError through the error chain.
	wrappedHTTPError := fmtWrapError{cause: direct}

	tests := []struct {
		name  string
		input error
		want  *HTTPError
	}{
		{name: "nil", input: nil, want: nil},
		{name: "direct HTTPError", input: direct, want: direct},
		{name: "wrapped HTTPError", input: wrappedHTTPError, want: direct},
		{
			name:  "unrelated error",
			input: errors.New("database unavailable"),
			want: &HTTPError{
				Status:  http.StatusInternalServerError,
				Message: "Internal Server Error",
				Code:    "internal",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AsHTTPError(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("AsHTTPError() = %#v, want %#v", got, tt.want)
			}
		})
	}

}

// fmtWrapError is a minimal error wrapper used to verify that AsHTTPError
// follows the standard Go error chain.
type fmtWrapError struct {
	cause error
}

func (e fmtWrapError) Error() string {
	return "wrapped: " + e.cause.Error()
}

func (e fmtWrapError) Unwrap() error {
	return e.cause
}
