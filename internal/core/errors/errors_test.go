package errors

import (
	"errors"
	"net/http"
	"testing"
)

func TestNewHTTPError(test *testing.T) {
	httpError := New(http.StatusBadRequest, "email is required")
	if httpError.Status != http.StatusBadRequest || httpError.Message != "email is required" || httpError.Code != "" || httpError.Details != nil {
		test.Fatalf("New() = %#v", httpError)
	}
}

func TestHTTPErrorStringIncludesCodeWhenPresent(test *testing.T) {
	withoutCode := New(http.StatusBadRequest, "missing")
	if withoutCode.Error() != "400: missing" {
		test.Fatalf("Error() without code = %q", withoutCode.Error())
	}

	withCode := NotFound("user not found")
	if withCode.Error() != "404 not_found: user not found" {
		test.Fatalf("Error() with code = %q", withCode.Error())
	}
}

func TestNotFoundUsesDefaultAndCustomMessages(test *testing.T) {
	defaultError := NotFound("")
	if defaultError.Status != http.StatusNotFound || defaultError.Code != "not_found" || defaultError.Message != "Not Found" {
		test.Fatalf("empty NotFound = %#v", defaultError)
	}

	customError := NotFound("widget missing")
	if customError.Message != "widget missing" || customError.Code != "not_found" {
		test.Fatalf("custom NotFound = %#v", customError)
	}
}

func TestMethodNotAllowedIncludesAllowDetails(test *testing.T) {
	httpError := MethodNotAllowed("GET, POST")
	if httpError.Status != http.StatusMethodNotAllowed || httpError.Code != "method_not_allowed" {
		test.Fatalf("MethodNotAllowed = %#v", httpError)
	}
	if httpError.Message != "Method Not Allowed" {
		test.Fatalf("message = %q", httpError.Message)
	}
	if httpError.Details["allow"] != "GET, POST" {
		test.Fatalf("details = %#v", httpError.Details)
	}
	if httpError.Error() != "405 method_not_allowed: Method Not Allowed" {
		test.Fatalf("Error() = %q", httpError.Error())
	}
}

func TestAsHTTPErrorHidesUnknownErrors(test *testing.T) {
	value := AsHTTPError(errors.New("database password"))
	if value.Status != http.StatusInternalServerError || value.Code != "internal" || value.Message != "Internal Server Error" {
		test.Fatalf("value = %#v", value)
	}
}

func TestAsHTTPErrorPreservesWrappedHTTPError(test *testing.T) {
	original := New(http.StatusTeapot, "short and stout")
	wrapped := errors.Join(errors.New("wrapper"), original)
	if convertedError := AsHTTPError(wrapped); convertedError != original {
		test.Fatalf("converted %#v, want original", convertedError)
	}
}

func TestAsHTTPErrorPreservesBareHTTPError(test *testing.T) {
	original := NotFound("missing")
	if convertedError := AsHTTPError(original); convertedError != original {
		test.Fatalf("converted %#v, want original", convertedError)
	}
}

func TestAsHTTPErrorNil(test *testing.T) {
	if AsHTTPError(nil) != nil {
		test.Fatal("AsHTTPError(nil) should return nil")
	}
}
