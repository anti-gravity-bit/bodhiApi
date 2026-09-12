package bodhiApi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// TestNewContextInitializesRequestState verifies that new contexts retain the
// HTTP exchange, normalize nil parameters, and start with an empty store.
func TestNewContextInitializesRequestState(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/users?active=true", nil)
	writer := httptest.NewRecorder()
	context := newContext(writer, request, nil)

	if context.Request() != request {
		t.Fatal("Request() should return the original request")
	}
	if context.ResponseWriter() != writer {
		t.Fatal("ResponseWriter() should return the original writer")
	}
	if got := context.Param("missing"); got != "" {
		t.Fatalf("missing Param() = %q, want empty string", got)
	}
	if value, ok := context.Get("missing"); ok || value != nil {
		t.Fatalf("missing Get() = (%#v, %v), want (nil, false)", value, ok)
	}
}

// TestContextReadsRequestValues verifies route parameters, query values, and
// headers are read from their respective request sources.
func TestContextReadsRequestValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/users/42?active=true", nil)
	request.Header.Set("X-Trace-ID", "trace-123")
	context := newContext(httptest.NewRecorder(), request, map[string]string{"id": "42"})

	if got := context.Param("id"); got != "42" {
		t.Fatalf("Param(id) = %q, want 42", got)
	}
	if got := context.Query("active"); got != "true" {
		t.Fatalf("Query(active) = %q, want true", got)
	}
	if got := context.Header("X-Trace-ID"); got != "trace-123" {
		t.Fatalf("Header(X-Trace-ID) = %q, want trace-123", got)
	}
}

// TestContextStore verifies that Set and Get share request-scoped values and
// distinguish a missing key from a stored nil value.
func TestContextStore(t *testing.T) {
	context := newContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), nil)
	context.Set("user", "alice")
	context.Set("nil-value", nil)

	if value, ok := context.Get("user"); !ok || value != "alice" {
		t.Fatalf("Get(user) = (%#v, %v), want (alice, true)", value, ok)
	}
	if value, ok := context.Get("nil-value"); !ok || value != nil {
		t.Fatalf("Get(nil-value) = (%#v, %v), want (nil, true)", value, ok)
	}
}

// TestContextJSON writes and decodes a JSON response, verifying its status,
// content type, and body.
func TestContextJSON(t *testing.T) {
	writer := httptest.NewRecorder()
	context := newContext(writer, httptest.NewRequest(http.MethodGet, "/", nil), nil)

	if err := context.JSON(http.StatusCreated, map[string]string{"message": "created"}); err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	response := writer.Result()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusCreated)
	}
	if got := response.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON content type", got)
	}

	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON body: %v", err)
	}
	if !reflect.DeepEqual(body, map[string]string{"message": "created"}) {
		t.Fatalf("body = %#v, want message=created", body)
	}
	if !context.wrote {
		t.Fatal("JSON() should mark the response as written")
	}
}

// TestContextStatus writes only a status code and marks the response as
// written.
func TestContextStatus(t *testing.T) {
	writer := httptest.NewRecorder()
	context := newContext(writer, httptest.NewRequest(http.MethodGet, "/", nil), nil)

	if err := context.Status(http.StatusNoContent); err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}
	if writer.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", writer.Code, http.StatusNoContent)
	}
	if body, err := io.ReadAll(writer.Result().Body); err != nil || len(body) != 0 {
		t.Fatalf("status response body = %q, want empty", body)
	}
	if !context.wrote {
		t.Fatal("Status() should mark the response as written")
	}
}
