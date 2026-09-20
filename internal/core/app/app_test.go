package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
)

func TestNewConfiguresHealthRouteAndDefaults(test *testing.T) {
	application := New(Info{Title: "Example", Version: "1.0.0"})
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if responseRecorder.Code != http.StatusOK {
		test.Fatalf("health status = %d", responseRecorder.Code)
	}
	if !strings.Contains(responseRecorder.Body.String(), `"status":"ok"`) {
		test.Fatalf("health body = %q", responseRecorder.Body.String())
	}
	if responseRecorder.Header().Get("X-Request-ID") == "" {
		test.Fatal("health response lacks request ID")
	}
}

func TestApplicationReturnsNotFoundAndMethodNotAllowed(test *testing.T) {
	application := New(Info{})
	application.GET("/users", func(requestContext core.Context) error {
		return requestContext.Status(http.StatusNoContent)
	})

	missingResponse := httptest.NewRecorder()
	application.Handler().ServeHTTP(missingResponse, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if missingResponse.Code != http.StatusNotFound {
		test.Fatalf("missing status = %d", missingResponse.Code)
	}
	if !strings.Contains(missingResponse.Body.String(), `"code":"not_found"`) {
		test.Fatalf("missing body = %q", missingResponse.Body.String())
	}

	methodResponse := httptest.NewRecorder()
	application.Handler().ServeHTTP(methodResponse, httptest.NewRequest(http.MethodPost, "/users", nil))
	if methodResponse.Code != http.StatusMethodNotAllowed {
		test.Fatalf("method status = %d", methodResponse.Code)
	}
	if methodResponse.Header().Get("Allow") != http.MethodGet {
		test.Fatalf("Allow = %q", methodResponse.Header().Get("Allow"))
	}
	if !strings.Contains(methodResponse.Body.String(), `"code":"method_not_allowed"`) {
		test.Fatalf("method body = %q", methodResponse.Body.String())
	}
	if missingResponse.Header().Get("Allow") != "" {
		test.Fatalf("404 should not set Allow: %q", missingResponse.Header().Get("Allow"))
	}
}

func TestApplicationSetsAllowHeaderOnMethodNotAllowed(test *testing.T) {
	application := New(Info{})
	application.GET("/users", func(requestContext core.Context) error { return requestContext.Status(http.StatusNoContent) })
	application.POST("/users", func(requestContext core.Context) error { return requestContext.Status(http.StatusCreated) })

	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodDelete, "/users", nil))
	if responseRecorder.Code != http.StatusMethodNotAllowed {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRecorder.Header().Get("Allow") != "GET, POST" {
		test.Fatalf("Allow = %q", responseRecorder.Header().Get("Allow"))
	}
}

func TestApplicationGroupRegistersPrefixedRoute(test *testing.T) {
	application := New(Info{})
	application.Group("/v1").GET("/users", func(requestContext core.Context) error {
		return requestContext.Status(http.StatusNoContent)
	})

	matchedRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(matchedRecorder, httptest.NewRequest(http.MethodGet, "/v1/users", nil))
	if matchedRecorder.Code != http.StatusNoContent {
		test.Fatalf("grouped status = %d", matchedRecorder.Code)
	}

	missingRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(missingRecorder, httptest.NewRequest(http.MethodGet, "/users", nil))
	if missingRecorder.Code != http.StatusNotFound {
		test.Fatalf("unprefixed status = %d", missingRecorder.Code)
	}
}

func TestApplicationDoesNotOverwriteWrittenResponse(test *testing.T) {
	application := New(Info{})
	application.GET("/written", func(requestContext core.Context) error {
		_ = requestContext.Status(http.StatusAccepted)
		return errors.New("late error")
	})
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/written", nil))
	if responseRecorder.Code != http.StatusAccepted {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRecorder.Body.Len() != 0 {
		test.Fatalf("body = %q", responseRecorder.Body.String())
	}
}

func TestApplicationWritesReturnedErrorWhenNothingWasWritten(test *testing.T) {
	application := New(Info{})
	application.GET("/fail", func(core.Context) error { return errors.New("database password") })
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/fail", nil))
	if responseRecorder.Code != http.StatusInternalServerError {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	if strings.Contains(responseRecorder.Body.String(), "database password") {
		test.Fatalf("leaked internal error: %q", responseRecorder.Body.String())
	}
	if !strings.Contains(responseRecorder.Body.String(), `"code":"internal"`) {
		test.Fatalf("body = %q", responseRecorder.Body.String())
	}
}

func TestApplicationRecoversFromHandlerPanic(test *testing.T) {
	application := New(Info{})
	application.GET("/panic", func(core.Context) error { panic("unexpected") })
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if responseRecorder.Code != http.StatusInternalServerError {
		test.Fatalf("status = %d body = %q", responseRecorder.Code, responseRecorder.Body.String())
	}
}

func TestApplicationMethodHelpersAndRouterAccessor(test *testing.T) {
	application := New(Info{})
	if application.Router() == nil {
		test.Fatal("Router() returned nil")
	}

	application.POST("/items", func(requestContext core.Context) error { return requestContext.Status(http.StatusCreated) })
	application.PUT("/items", func(requestContext core.Context) error { return requestContext.Status(http.StatusOK) })
	application.PATCH("/items", func(requestContext core.Context) error { return requestContext.Status(http.StatusNoContent) })
	application.DELETE("/items", func(requestContext core.Context) error { return requestContext.Status(http.StatusNoContent) })

	methodCases := []struct {
		method         string
		expectedStatus int
	}{
		{method: http.MethodPost, expectedStatus: http.StatusCreated},
		{method: http.MethodPut, expectedStatus: http.StatusOK},
		{method: http.MethodPatch, expectedStatus: http.StatusNoContent},
		{method: http.MethodDelete, expectedStatus: http.StatusNoContent},
	}
	for _, methodCase := range methodCases {
		responseRecorder := httptest.NewRecorder()
		application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(methodCase.method, "/items", nil))
		if responseRecorder.Code != methodCase.expectedStatus {
			test.Fatalf("%s status = %d, want %d", methodCase.method, responseRecorder.Code, methodCase.expectedStatus)
		}
	}
}

func TestApplicationUseRunsOnUnmatchedRoutes(test *testing.T) {
	application := New(Info{})
	middlewareRan := false
	application.Use(func(nextHandler core.Handler) core.Handler {
		return func(requestContext core.Context) error {
			middlewareRan = true
			return nextHandler(requestContext)
		}
	})

	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if !middlewareRan {
		test.Fatal("middleware did not run for unmatched route")
	}
	if responseRecorder.Code != http.StatusNotFound {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestApplicationRouteParametersAndQuery(test *testing.T) {
	application := New(Info{})
	application.GET("/users/:id/posts/:postId", func(requestContext core.Context) error {
		return requestContext.JSON(http.StatusOK, map[string]string{
			"id":     requestContext.Param("id"),
			"postId": requestContext.Param("postId"),
			"sort":   requestContext.Query("sort"),
		})
	})
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/users/1/posts/9?sort=asc", nil))
	if responseRecorder.Code != http.StatusOK {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	var responseBody map[string]string
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &responseBody); err != nil {
		test.Fatal(err)
	}
	if responseBody["id"] != "1" || responseBody["postId"] != "9" || responseBody["sort"] != "asc" {
		test.Fatalf("body = %#v", responseBody)
	}
}

func TestApplicationReusesIncomingRequestID(test *testing.T) {
	application := New(Info{})
	httpRequest := httptest.NewRequest(http.MethodGet, "/health", nil)
	httpRequest.Header.Set("X-Request-ID", "incoming-id")
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Header().Get("X-Request-ID") != "incoming-id" {
		test.Fatalf("request ID = %q", responseRecorder.Header().Get("X-Request-ID"))
	}
}

func TestShutdownBeforeStart(test *testing.T) {
	if err := New(Info{}).Shutdown(context.Background()); err != nil {
		test.Fatal(err)
	}
}

func TestNormalizeServerError(test *testing.T) {
	if err := normalizeServerError(http.ErrServerClosed); err != nil {
		test.Fatalf("normalized close error = %v", err)
	}
	originalError := errors.New("server failed")
	if normalizedError := normalizeServerError(originalError); normalizedError != originalError {
		test.Fatal("non-close error was changed")
	}
	if err := normalizeServerError(nil); err != nil {
		test.Fatalf("nil error = %v", err)
	}
}

func TestNewServerConfiguration(test *testing.T) {
	application := New(Info{})
	server := newServer(application, ":8080")
	if server.Addr != ":8080" || server.Handler == nil {
		test.Fatalf("server = %#v", server)
	}
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 15*time.Second || server.WriteTimeout != 15*time.Second || server.IdleTimeout != 60*time.Second {
		test.Fatalf("timeouts = %#v", server)
	}
	if server.MaxHeaderBytes != 1<<20 {
		test.Fatalf("MaxHeaderBytes = %d", server.MaxHeaderBytes)
	}
}

func TestListenAndShutdown(test *testing.T) {
	application := New(Info{})
	networkListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	serverErrorChannel := make(chan error, 1)
	go func() {
		serverErrorChannel <- application.Listen(networkListener)
	}()

	healthURL := "http://" + networkListener.Addr().String() + "/health"
	deadline := time.Now().Add(2 * time.Second)
	for {
		healthResponse, healthError := http.Get(healthURL)
		if healthError == nil {
			_, _ = io.Copy(io.Discard, healthResponse.Body)
			_ = healthResponse.Body.Close()
			if healthResponse.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			test.Fatalf("server was not reachable: %v", healthError)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := application.Shutdown(context.Background()); err != nil {
		test.Fatal(err)
	}
	if err := <-serverErrorChannel; err != nil {
		test.Fatal(err)
	}
}

func TestListenAndServeOnBusyAddress(test *testing.T) {
	busyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	defer busyListener.Close()

	application := New(Info{Title: "busy"})
	serveError := application.ListenAndServe(busyListener.Addr().String())
	if serveError == nil {
		test.Fatal("expected listen error for an address that is already in use")
	}
}

func TestWriteErrorEncodesHTTPError(test *testing.T) {
	responseRecorder := httptest.NewRecorder()
	writeError(responseRecorder, errors.New("secret"))
	if responseRecorder.Code != http.StatusInternalServerError {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRecorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		test.Fatalf("content type = %q", responseRecorder.Header().Get("Content-Type"))
	}
	if strings.Contains(responseRecorder.Body.String(), "secret") {
		test.Fatalf("leaked error: %q", responseRecorder.Body.String())
	}
}

func TestApplicationInfoIsStored(test *testing.T) {
	application := New(Info{Title: "Users API", Version: "1.2.3"})
	if application.Info.Title != "Users API" || application.Info.Version != "1.2.3" {
		test.Fatalf("info = %#v", application.Info)
	}
}

func TestApplicationRouterRegistrationIsShared(test *testing.T) {
	application := New(Info{})
	application.Router().GET("/direct", func(requestContext core.Context) error {
		return requestContext.Status(http.StatusTeapot)
	})
	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/direct", nil))
	if responseRecorder.Code != http.StatusTeapot {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
}
