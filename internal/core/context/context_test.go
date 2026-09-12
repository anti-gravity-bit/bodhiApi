package context

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRequestContextStoresRequestValues(test *testing.T) {
	httpRequest := httptest.NewRequest(http.MethodGet, "/users/42?active=true", nil)
	httpRequest.Header.Set("X-Trace-ID", "trace-123")
	requestContext := New(httptest.NewRecorder(), httpRequest, map[string]string{"id": "42"})

	if requestContext.Request() != httpRequest {
		test.Fatal("Request did not return the original request")
	}
	if requestContext.Param("id") != "42" {
		test.Fatalf("Param = %q", requestContext.Param("id"))
	}
	if requestContext.Param("missing") != "" {
		test.Fatal("missing parameters should be empty")
	}
	if requestContext.Query("active") != "true" {
		test.Fatalf("Query = %q", requestContext.Query("active"))
	}
	if requestContext.Query("missing") != "" {
		test.Fatal("missing query values should be empty")
	}
	if requestContext.Header("X-Trace-ID") != "trace-123" {
		test.Fatalf("Header = %q", requestContext.Header("X-Trace-ID"))
	}
	if requestContext.Header("x-trace-id") != "trace-123" {
		test.Fatal("header lookup should be canonical")
	}
	if requestContext.Header("missing") != "" {
		test.Fatal("missing headers should be empty")
	}
	if requestContext.ResponseWriter() == nil {
		test.Fatal("ResponseWriter should not be nil")
	}
	if requestContext.Wrote() {
		test.Fatal("new context should not be marked written")
	}
}

func TestRequestContextAcceptsNilParameterMap(test *testing.T) {
	requestContext := New(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), nil)
	if requestContext.Param("id") != "" {
		test.Fatal("nil parameter map should behave like an empty map")
	}
}

func TestRequestContextStoreDistinguishesMissingAndNil(test *testing.T) {
	requestContext := New(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), nil)
	requestContext.Set("user", "alice")
	requestContext.Set("nil-value", nil)

	storedValue, found := requestContext.Get("user")
	if !found || storedValue != "alice" {
		test.Fatalf("stored value = %#v, %v", storedValue, found)
	}
	storedValue, found = requestContext.Get("nil-value")
	if !found || storedValue != nil {
		test.Fatalf("nil value = %#v, %v", storedValue, found)
	}
	storedValue, found = requestContext.Get("missing")
	if found || storedValue != nil {
		test.Fatalf("missing value = %#v, %v", storedValue, found)
	}

	requestContext.Set("user", "bob")
	storedValue, found = requestContext.Get("user")
	if !found || storedValue != "bob" {
		test.Fatalf("overwritten value = %#v, %v", storedValue, found)
	}
}

func TestRequestContextJSONWritesResponse(test *testing.T) {
	responseRecorder := httptest.NewRecorder()
	requestContext := New(responseRecorder, httptest.NewRequest(http.MethodGet, "/", nil), nil)

	if err := requestContext.JSON(http.StatusCreated, map[string]string{"message": "created"}); err != nil {
		test.Fatal(err)
	}
	if responseRecorder.Code != http.StatusCreated {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRecorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		test.Fatal("JSON content type missing")
	}
	var responseBody map[string]string
	if err := json.NewDecoder(responseRecorder.Result().Body).Decode(&responseBody); err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(responseBody, map[string]string{"message": "created"}) {
		test.Fatalf("body = %#v", responseBody)
	}
	if !requestContext.Wrote() {
		test.Fatal("JSON should mark context as written")
	}
}

func TestRequestContextJSONEncodesNilAsJSONNull(test *testing.T) {
	responseRecorder := httptest.NewRecorder()
	requestContext := New(responseRecorder, httptest.NewRequest(http.MethodGet, "/", nil), nil)
	if err := requestContext.JSON(http.StatusOK, nil); err != nil {
		test.Fatal(err)
	}
	if strings.TrimSpace(responseRecorder.Body.String()) != "null" {
		test.Fatalf("body = %q", responseRecorder.Body.String())
	}
}

func TestRequestContextJSONReturnsEncodeErrorAfterMarkingWritten(test *testing.T) {
	responseRecorder := httptest.NewRecorder()
	requestContext := New(responseRecorder, httptest.NewRequest(http.MethodGet, "/", nil), nil)
	encodeError := requestContext.JSON(http.StatusOK, make(chan int))
	if encodeError == nil {
		test.Fatal("expected JSON encode error for a channel value")
	}
	if !requestContext.Wrote() {
		test.Fatal("failed JSON write should still mark the response as written")
	}
	if responseRecorder.Code != http.StatusOK {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestRequestContextStatusWritesEmptyResponse(test *testing.T) {
	responseRecorder := httptest.NewRecorder()
	requestContext := New(responseRecorder, httptest.NewRequest(http.MethodGet, "/", nil), nil)

	if err := requestContext.Status(http.StatusNoContent); err != nil {
		test.Fatal(err)
	}
	if responseRecorder.Code != http.StatusNoContent {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	responseBody, err := io.ReadAll(responseRecorder.Result().Body)
	if err != nil {
		test.Fatal(err)
	}
	if len(responseBody) != 0 {
		test.Fatalf("body = %q", responseBody)
	}
	if !requestContext.Wrote() {
		test.Fatal("Status should mark context as written")
	}
}

func TestRequestContextQueryReturnsFirstValue(test *testing.T) {
	httpRequest := httptest.NewRequest(http.MethodGet, "/search?tag=one&tag=two", nil)
	requestContext := New(httptest.NewRecorder(), httpRequest, nil)
	if requestContext.Query("tag") != "one" {
		test.Fatalf("Query = %q", requestContext.Query("tag"))
	}
}
