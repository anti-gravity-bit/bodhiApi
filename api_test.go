package bodhiApi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicApplicationAPI(test *testing.T) {
	application := New(Info{Title: "Example", Version: "1.0.0"})
	application.GET("/users/:id", func(requestContext Context) error {
		return requestContext.JSON(http.StatusOK, map[string]string{"id": requestContext.Param("id")})
	})

	httpRequest := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httpRequest)

	if responseRecorder.Code != http.StatusOK || !strings.Contains(responseRecorder.Body.String(), `"id":"42"`) {
		test.Fatalf("response = %d %q", responseRecorder.Code, responseRecorder.Body.String())
	}
	if responseRecorder.Header().Get(HeaderRequestID) == "" {
		test.Fatal("request ID was not added")
	}
}

func TestPublicApplicationMethodHelpers(test *testing.T) {
	application := New(Info{Title: "Methods", Version: "1.0.0"})
	application.POST("/items", func(requestContext Context) error { return requestContext.Status(http.StatusCreated) })
	application.PUT("/items/:id", func(requestContext Context) error {
		return requestContext.JSON(http.StatusOK, map[string]string{"id": requestContext.Param("id")})
	})
	application.PATCH("/items/:id", func(requestContext Context) error { return requestContext.Status(http.StatusNoContent) })
	application.DELETE("/items/:id", func(requestContext Context) error { return requestContext.Status(http.StatusNoContent) })

	methodCases := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{name: "POST", method: http.MethodPost, path: "/items", expectedStatus: http.StatusCreated},
		{name: "PUT", method: http.MethodPut, path: "/items/7", expectedStatus: http.StatusOK},
		{name: "PATCH", method: http.MethodPatch, path: "/items/7", expectedStatus: http.StatusNoContent},
		{name: "DELETE", method: http.MethodDelete, path: "/items/7", expectedStatus: http.StatusNoContent},
	}
	for _, methodCase := range methodCases {
		test.Run(methodCase.name, func(subtest *testing.T) {
			responseRecorder := httptest.NewRecorder()
			application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(methodCase.method, methodCase.path, nil))
			if responseRecorder.Code != methodCase.expectedStatus {
				subtest.Fatalf("status = %d, want %d", responseRecorder.Code, methodCase.expectedStatus)
			}
		})
	}
}

func TestPublicApplicationUseMiddleware(test *testing.T) {
	application := New(Info{})
	application.Use(func(nextHandler Handler) Handler {
		return func(requestContext Context) error {
			requestContext.Set("seen", true)
			return nextHandler(requestContext)
		}
	})
	application.GET("/flag", func(requestContext Context) error {
		value, found := requestContext.Get("seen")
		if !found || value != true {
			test.Fatal("application middleware did not run")
		}
		return requestContext.Status(http.StatusNoContent)
	})

	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/flag", nil))
	if responseRecorder.Code != http.StatusNoContent {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestPublicErrors(test *testing.T) {
	notFoundError := NotFound("")
	if notFoundError.Code != "not_found" || notFoundError.Status != http.StatusNotFound {
		test.Fatalf("NotFound() = %#v", notFoundError)
	}
	if notFoundError.Message != "Not Found" {
		test.Fatalf("NotFound empty message = %q", notFoundError.Message)
	}

	customNotFound := NotFound("user missing")
	if customNotFound.Message != "user missing" {
		test.Fatalf("NotFound custom message = %q", customNotFound.Message)
	}

	methodError := MethodNotAllowed("GET, POST")
	if methodError.Status != http.StatusMethodNotAllowed || methodError.Code != "method_not_allowed" {
		test.Fatalf("MethodNotAllowed() = %#v", methodError)
	}
	if methodError.Details["allow"] != "GET, POST" {
		test.Fatalf("MethodNotAllowed details = %#v", methodError.Details)
	}

	constructedError := NewHTTPError(http.StatusBadRequest, "email is required")
	if constructedError.Status != http.StatusBadRequest || constructedError.Message != "email is required" || constructedError.Code != "" {
		test.Fatalf("NewHTTPError() = %#v", constructedError)
	}

	hiddenError := AsHTTPError(errors.New("secret"))
	if hiddenError.Code != "internal" || hiddenError.Status != http.StatusInternalServerError {
		test.Fatalf("AsHTTPError() = %#v", hiddenError)
	}
	if AsHTTPError(nil) != nil {
		test.Fatal("AsHTTPError(nil) should be nil")
	}
}

func TestPublicRouterAndMiddleware(test *testing.T) {
	routerInstance := NewRouter()
	routerInstance.Use(func(nextHandler Handler) Handler {
		return func(requestContext Context) error {
			requestContext.Set("seen", true)
			return nextHandler(requestContext)
		}
	})
	routerInstance.GET("/health", func(requestContext Context) error {
		value, found := requestContext.Get("seen")
		if !found || value != true {
			test.Fatal("middleware did not run")
		}
		return requestContext.Status(http.StatusNoContent)
	})

	httpRequest := httptest.NewRequest(http.MethodGet, "/health", nil)
	responseRecorder := httptest.NewRecorder()
	routerHandler := routerInstance.Handler()
	routerHandler.ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusNoContent {
		test.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusNoContent)
	}
}

func TestPublicRecoverConvertsPanics(test *testing.T) {
	handlerError := Recover(func(Context) error { panic("boom") })(newStubContext())
	convertedError := AsHTTPError(handlerError)
	if convertedError.Status != http.StatusInternalServerError {
		test.Fatalf("Recover error = %#v", convertedError)
	}
}

func TestPublicRecoverPassesThroughHandlerError(test *testing.T) {
	originalError := errors.New("handler failed")
	handlerError := Recover(func(Context) error { return originalError })(newStubContext())
	if handlerError != originalError {
		test.Fatalf("handler error = %v", handlerError)
	}
}

func TestPublicRequestIDReusesAndGenerates(test *testing.T) {
	incomingRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	incomingRequest.Header.Set(HeaderRequestID, "public-id")
	incomingRecorder := httptest.NewRecorder()
	incomingContext := newStubContextWith(incomingRecorder, incomingRequest)
	if err := RequestID(func(Context) error { return nil })(incomingContext); err != nil {
		test.Fatal(err)
	}
	if incomingRecorder.Header().Get(HeaderRequestID) != "public-id" {
		test.Fatalf("reused ID = %q", incomingRecorder.Header().Get(HeaderRequestID))
	}

	generatedRecorder := httptest.NewRecorder()
	generatedContext := newStubContextWith(generatedRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if err := RequestID(func(Context) error { return nil })(generatedContext); err != nil {
		test.Fatal(err)
	}
	generatedID := generatedRecorder.Header().Get(HeaderRequestID)
	if generatedID == "" {
		test.Fatal("generated request ID was empty")
	}
	storedValue, found := generatedContext.Get("request_id")
	if !found || storedValue != generatedID {
		test.Fatalf("stored request ID = %#v found=%v", storedValue, found)
	}
}

func TestPublicLoggerEmitsStructuredRecords(test *testing.T) {
	var logBuffer bytes.Buffer
	structuredLogger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	requestContext := newStubContext()
	middleware := Logger(structuredLogger)
	if err := middleware(func(Context) error { return errors.New("failed") })(requestContext); err == nil {
		test.Fatal("expected handler error")
	}
	if !strings.Contains(logBuffer.String(), `"err":"failed"`) {
		test.Fatalf("log output = %q", logBuffer.String())
	}
}

func TestPublicLoggerNilLoggerDoesNotPanic(test *testing.T) {
	middleware := Logger(nil)
	if err := middleware(func(Context) error { return nil })(newStubContext()); err != nil {
		test.Fatal(err)
	}
}

func TestShutdownBeforeStart(test *testing.T) {
	if err := New(Info{}).Shutdown(context.Background()); err != nil {
		test.Fatal(err)
	}
}

func TestPublicHeaderRequestIDConstant(test *testing.T) {
	if HeaderRequestID != "X-Request-ID" {
		test.Fatalf("HeaderRequestID = %q", HeaderRequestID)
	}
}

func TestPublicContextThroughHandler(test *testing.T) {
	application := New(Info{})
	application.GET("/inspect", func(requestContext Context) error {
		if requestContext.Request() == nil || requestContext.ResponseWriter() == nil {
			test.Fatal("request or writer missing")
		}
		if requestContext.Query("active") != "true" {
			test.Fatalf("query = %q", requestContext.Query("active"))
		}
		if requestContext.Header("X-Trace") != "abc" {
			test.Fatalf("header = %q", requestContext.Header("X-Trace"))
		}
		requestContext.Set("user", "ada")
		value, found := requestContext.Get("user")
		if !found || value != "ada" {
			test.Fatalf("store = %#v %v", value, found)
		}
		return requestContext.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	httpRequest := httptest.NewRequest(http.MethodGet, "/inspect?active=true", nil)
	httpRequest.Header.Set("X-Trace", "abc")
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusOK {
		test.Fatalf("status = %d %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var responseBody map[string]string
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &responseBody); err != nil {
		test.Fatal(err)
	}
	if responseBody["ok"] != "true" {
		test.Fatalf("body = %#v", responseBody)
	}
}

// stubContext is a Context implementation for public middleware tests. It avoids
// depending on unexported fields of the internal RequestContext type.
type stubContext struct {
	httpRequest    *http.Request
	responseWriter http.ResponseWriter
	requestValues  map[string]any
}

func newStubContext() *stubContext {
	return newStubContextWith(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func newStubContextWith(responseWriter http.ResponseWriter, httpRequest *http.Request) *stubContext {
	return &stubContext{httpRequest: httpRequest, responseWriter: responseWriter, requestValues: map[string]any{}}
}

func (stub *stubContext) Request() *http.Request              { return stub.httpRequest }
func (stub *stubContext) ResponseWriter() http.ResponseWriter { return stub.responseWriter }
func (stub *stubContext) Param(string) string                 { return "" }
func (stub *stubContext) Query(name string) string            { return stub.httpRequest.URL.Query().Get(name) }
func (stub *stubContext) Header(name string) string           { return stub.httpRequest.Header.Get(name) }
func (stub *stubContext) JSON(int, any) error                 { return nil }
func (stub *stubContext) Status(int) error                    { return nil }
func (stub *stubContext) Set(key string, value any)           { stub.requestValues[key] = value }
func (stub *stubContext) Get(key string) (any, bool) {
	value, found := stub.requestValues[key]
	return value, found
}
