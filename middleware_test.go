package bodhiApi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// middlewareTestContext is a small Context implementation for middleware
// tests. It records only the interactions middleware is expected to perform.
type middlewareTestContext struct {
	request        *http.Request
	writer         *httptest.ResponseRecorder
	requestHeaders map[string]string
	values         map[string]any
}

func newMiddlewareTestContext(request *http.Request) *middlewareTestContext {
	return &middlewareTestContext{
		request:        request,
		writer:         httptest.NewRecorder(),
		requestHeaders: map[string]string{},
		values:         map[string]any{},
	}
}

func (c *middlewareTestContext) Request() *http.Request {
	return c.request
}

func (c *middlewareTestContext) ResponseWriter() http.ResponseWriter {
	return c.writer
}

func (c *middlewareTestContext) Param(string) string { return "" }
func (c *middlewareTestContext) Query(string) string { return "" }

func (c *middlewareTestContext) Header(name string) string {
	return c.requestHeaders[name]
}

func (c *middlewareTestContext) JSON(int, any) error { return nil }
func (c *middlewareTestContext) Status(int) error    { return nil }

func (c *middlewareTestContext) Set(key string, value any) {
	c.values[key] = value
}

func (c *middlewareTestContext) Get(key string) (any, bool) {
	value, ok := c.values[key]
	return value, ok
}

// TestRecoverConvertsPanic verifies that a downstream panic becomes an
// internal-server-error HTTPError and is not rethrown.
func TestRecoverConvertsPanic(t *testing.T) {
	context := newMiddlewareTestContext(httptest.NewRequest(http.MethodGet, "/", nil))
	wrapped := Recover(func(Context) error {
		panic("unexpected failure")
	})

	err := wrapped(context)
	if err == nil {
		t.Fatal("Recover() should return an error after a panic")
	}

	httpError, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if httpError.Status != http.StatusInternalServerError || httpError.Message != "internal server error" {
		t.Fatalf("error = %#v, want internal server error", httpError)
	}
}

// TestRecoverPassesThroughNormalResults verifies that non-panicking handlers'
// errors are returned unchanged.
func TestRecoverPassesThroughNormalResults(t *testing.T) {
	context := newMiddlewareTestContext(httptest.NewRequest(http.MethodGet, "/", nil))
	want := errors.New("handler failed")
	wrapped := Recover(func(Context) error { return want })

	if got := wrapped(context); got != want {
		t.Fatalf("Recover() error = %v, want original error", got)
	}
}

// TestRequestIDUsesIncomingID verifies that a supplied request ID is reused in
// Context and echoed in the response.
func TestRequestIDUsesIncomingID(t *testing.T) {
	context := newMiddlewareTestContext(httptest.NewRequest(http.MethodGet, "/", nil))
	context.requestHeaders[HeaderRequestID] = "client-request-id"

	called := false
	wrapped := RequestID(func(c Context) error {
		called = true
		value, ok := c.Get(ctxKeyRequestID)
		if !ok || value != "client-request-id" {
			t.Fatalf("stored request ID = (%#v, %v), want client-request-id", value, ok)
		}
		return nil
	})

	if err := wrapped(context); err != nil {
		t.Fatalf("RequestID() returned error: %v", err)
	}
	if !called {
		t.Fatal("RequestID() should call the next handler")
	}
	if got := context.writer.Header().Get(HeaderRequestID); got != "client-request-id" {
		t.Fatalf("response request ID = %q, want client-request-id", got)
	}
}

// TestRequestIDGeneratesAndStoresID verifies the generated ID format when the
// client does not provide one.
func TestRequestIDGeneratesAndStoresID(t *testing.T) {
	context := newMiddlewareTestContext(httptest.NewRequest(http.MethodGet, "/", nil))
	wrapped := RequestID(func(c Context) error { return nil })

	if err := wrapped(context); err != nil {
		t.Fatalf("RequestID() returned error: %v", err)
	}

	value, ok := context.Get(ctxKeyRequestID)
	if !ok {
		t.Fatal("RequestID() should store the generated ID")
	}
	requestID, ok := value.(string)
	if !ok {
		t.Fatalf("request ID type = %T, want string", value)
	}
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(requestID) {
		t.Fatalf("generated request ID = %q, want 16 lowercase hex characters", requestID)
	}
	if got := context.writer.Header().Get(HeaderRequestID); got != requestID {
		t.Fatalf("response request ID = %q, want %q", got, requestID)
	}
}

// TestLoggerLogsSuccessAndError verifies log levels and the common request
// attributes emitted by Logger.
func TestLoggerLogsSuccessAndError(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	request := httptest.NewRequest(http.MethodPost, "/users", nil)

	context := newMiddlewareTestContext(request)
	context.Set(ctxKeyRequestID, "request-123")
	wrapped := Logger(logger)(func(Context) error { return nil })
	if err := wrapped(context); err != nil {
		t.Fatalf("successful Logger() call returned error: %v", err)
	}

	successLog := output.String()
	for _, expected := range []string{"level=INFO", "msg=request", "method=POST", "path=/users", "request_id=request-123"} {
		if !strings.Contains(successLog, expected) {
			t.Fatalf("success log %q does not contain %q", successLog, expected)
		}
	}

	output.Reset()
	context = newMiddlewareTestContext(request)
	wrapped = Logger(logger)(func(Context) error { return errors.New("database unavailable") })
	if err := wrapped(context); err == nil {
		t.Fatal("error handler should return its error")
	}

	errorLog := output.String()
	for _, expected := range []string{"level=ERROR", "msg=request", "err=\"database unavailable\""} {
		if !strings.Contains(errorLog, expected) {
			t.Fatalf("error log %q does not contain %q", errorLog, expected)
		}
	}
}

// TestLoggerUsesDefaultLogger verifies that a nil logger is accepted. The
// default logger is intentionally not inspected because it writes globally.
func TestLoggerUsesDefaultLogger(t *testing.T) {
	context := newMiddlewareTestContext(httptest.NewRequest(http.MethodGet, "/", nil))
	if err := Logger(nil)(func(Context) error { return nil })(context); err != nil {
		t.Fatalf("Logger(nil) returned error: %v", err)
	}
}

// TestNewRequestIDFallbackShape verifies that IDs generated under normal
// conditions have the documented compact hexadecimal shape.
func TestNewRequestIDFallbackShape(t *testing.T) {
	requestID := newRequestID()
	if requestID == "bodhiApi-unknown" {
		t.Skip("crypto/rand unavailable in this environment")
	}
	if len(requestID) != 16 {
		t.Fatalf("request ID length = %d, want 16", len(requestID))
	}
}
