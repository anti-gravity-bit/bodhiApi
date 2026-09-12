package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
	corecontext "github.com/anti-gravity-bit/bodhiApi/internal/core/context"
	coreerrors "github.com/anti-gravity-bit/bodhiApi/internal/core/errors"
)

func TestRequestIDReusesIncomingHeader(test *testing.T) {
	httpRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	httpRequest.Header.Set(HeaderRequestID, "request-123")
	responseRecorder := httptest.NewRecorder()
	requestContext := corecontext.New(responseRecorder, httpRequest, nil)

	if err := RequestID(func(core.Context) error { return nil })(requestContext); err != nil {
		test.Fatal(err)
	}
	if responseRecorder.Header().Get(HeaderRequestID) != "request-123" {
		test.Fatalf("request ID = %q", responseRecorder.Header().Get(HeaderRequestID))
	}
	storedValue, found := requestContext.Get(requestIDKey)
	if !found || storedValue != "request-123" {
		test.Fatalf("stored request ID = %#v found=%v", storedValue, found)
	}
}

func TestRequestIDGeneratesIdentifierWhenHeaderMissing(test *testing.T) {
	responseRecorder := httptest.NewRecorder()
	requestContext := corecontext.New(responseRecorder, httptest.NewRequest(http.MethodGet, "/", nil), nil)
	if err := RequestID(func(core.Context) error { return nil })(requestContext); err != nil {
		test.Fatal(err)
	}
	generatedID := responseRecorder.Header().Get(HeaderRequestID)
	if len(generatedID) != 16 {
		test.Fatalf("generated ID length = %d value = %q", len(generatedID), generatedID)
	}
	for _, character := range generatedID {
		if !isHexCharacter(character) {
			test.Fatalf("generated ID is not hex: %q", generatedID)
		}
	}
	storedValue, found := requestContext.Get(requestIDKey)
	if !found || storedValue != generatedID {
		test.Fatalf("stored request ID = %#v found=%v", storedValue, found)
	}
}

func TestRequestIDGeneratedValuesDiffer(test *testing.T) {
	firstID := newRequestID()
	secondID := newRequestID()
	if firstID == "" || secondID == "" {
		test.Fatal("generated request IDs should not be empty")
	}
	if firstID == secondID && firstID != "bodhiApi-unknown" {
		test.Fatal("successive request IDs should differ")
	}
}

func TestRecoverConvertsPanics(test *testing.T) {
	requestContext := corecontext.New(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), nil)
	handlerError := Recover(func(core.Context) error { panic("failure") })(requestContext)
	convertedError := coreerrors.AsHTTPError(handlerError)
	if convertedError.Status != http.StatusInternalServerError {
		test.Fatalf("error = %#v", convertedError)
	}
	if convertedError.Message != "internal server error" {
		test.Fatalf("message = %q", convertedError.Message)
	}
}

func TestRecoverPassesThroughNilAndErrors(test *testing.T) {
	requestContext := corecontext.New(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), nil)
	if err := Recover(func(core.Context) error { return nil })(requestContext); err != nil {
		test.Fatalf("nil return became %v", err)
	}

	originalError := errors.New("handler failed")
	handlerError := Recover(func(core.Context) error { return originalError })(requestContext)
	if handlerError != originalError {
		test.Fatalf("error return = %v", handlerError)
	}
}

func TestLoggerNilLoggerDoesNotPanic(test *testing.T) {
	requestContext := corecontext.New(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil), nil)
	if err := Logger(nil)(func(core.Context) error { return nil })(requestContext); err != nil {
		test.Fatal(err)
	}
}

func TestLoggerEmitsInfoOnSuccessAndErrorOnFailure(test *testing.T) {
	var logBuffer bytes.Buffer
	structuredLogger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	successContext := corecontext.New(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil), nil)
	if err := Logger(structuredLogger)(func(core.Context) error { return nil })(successContext); err != nil {
		test.Fatal(err)
	}
	if !strings.Contains(logBuffer.String(), `"level":"INFO"`) {
		test.Fatalf("success log = %q", logBuffer.String())
	}
	if !strings.Contains(logBuffer.String(), `"method":"GET"`) || !strings.Contains(logBuffer.String(), `"path":"/ok"`) {
		test.Fatalf("success log missing fields = %q", logBuffer.String())
	}

	logBuffer.Reset()
	errorContext := corecontext.New(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/fail", nil), nil)
	handlerError := Logger(structuredLogger)(func(core.Context) error { return errors.New("boom") })(errorContext)
	if handlerError == nil || handlerError.Error() != "boom" {
		test.Fatalf("handler error = %v", handlerError)
	}
	if !strings.Contains(logBuffer.String(), `"level":"ERROR"`) || !strings.Contains(logBuffer.String(), `"err":"boom"`) {
		test.Fatalf("error log = %q", logBuffer.String())
	}
}

func TestLoggerIncludesRequestIDWhenPresent(test *testing.T) {
	var logBuffer bytes.Buffer
	structuredLogger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	httpRequest := httptest.NewRequest(http.MethodGet, "/traced", nil)
	httpRequest.Header.Set(HeaderRequestID, "trace-id")
	requestContext := corecontext.New(httptest.NewRecorder(), httpRequest, nil)
	stackedHandler := RequestID(Logger(structuredLogger)(func(core.Context) error { return nil }))
	if err := stackedHandler(requestContext); err != nil {
		test.Fatal(err)
	}

	var logRecord map[string]any
	if err := json.Unmarshal(logBuffer.Bytes(), &logRecord); err != nil {
		test.Fatal(err)
	}
	if logRecord["request_id"] != "trace-id" {
		test.Fatalf("log record = %#v", logRecord)
	}
}

func TestLoggerOmitsNonStringRequestID(test *testing.T) {
	var logBuffer bytes.Buffer
	structuredLogger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	requestContext := corecontext.New(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), nil)
	requestContext.Set(requestIDKey, 42)
	if err := Logger(structuredLogger)(func(core.Context) error { return nil })(requestContext); err != nil {
		test.Fatal(err)
	}
	if strings.Contains(logBuffer.String(), "request_id") {
		test.Fatalf("non-string request ID should be omitted: %q", logBuffer.String())
	}
}

func TestHeaderRequestIDConstant(test *testing.T) {
	if HeaderRequestID != "X-Request-ID" {
		test.Fatalf("HeaderRequestID = %q", HeaderRequestID)
	}
}

func isHexCharacter(value rune) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f')
}
