package bodhiApi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestNewCreatesConfiguredApp verifies the default application setup,
// including the router, middleware stack, and health route.
func TestNewCreatesConfiguredApp(t *testing.T) {
	app := New(Info{Title: "Example", Version: "1.0.0"})

	if app.Info.Title != "Example" || app.Info.Version != "1.0.0" {
		t.Fatalf("Info = %#v, want configured application info", app.Info)
	}
	if app.Router() == nil {
		t.Fatal("Router() should return the application's router")
	}
	if len(app.router.stack) != 3 {
		t.Fatalf("default middleware count = %d, want 3", len(app.router.stack))
	}
	if app.log == nil {
		t.Fatal("New() should configure a logger")
	}

	response := executeRequest(app, http.MethodGet, "/health")
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get(HeaderRequestID); got == "" {
		t.Fatal("health response should contain a request ID")
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"status":"ok"}` {
		t.Fatalf("health body = %q, want status JSON", body)
	}
}

// TestAppRouteHelpersRegisterRoutes verifies that App forwards route
// registration to its underlying router.
func TestAppRouteHelpersRegisterRoutes(t *testing.T) {
	app := New(Info{})
	handler := func(Context) error { return nil }

	app.GET("/get", handler)
	app.POST("/post", handler)
	app.PUT("/put", handler)
	app.PATCH("/patch", handler)
	app.DELETE("/delete", handler)

	methods := make([]string, 0, len(app.router.routes))
	for _, route := range app.router.routes {
		if route.pattern == "/health" {
			continue
		}
		methods = append(methods, route.method)
	}

	want := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	if !reflect.DeepEqual(methods, want) {
		t.Fatalf("registered methods = %#v, want %#v", methods, want)
	}
}

// TestAppDispatchesRoute verifies that a registered route receives the
// request context and can write a response.
func TestAppDispatchesRoute(t *testing.T) {
	app := New(Info{})
	app.GET("/users/:id", func(c Context) error {
		return c.JSON(http.StatusAccepted, map[string]string{
			"id":    c.Param("id"),
			"query": c.Query("q"),
		})
	})

	response := executeRequest(app, http.MethodGet, "/users/42?q=active")
	if response.Code != http.StatusAccepted {
		t.Fatalf("route status = %d, want %d", response.Code, http.StatusAccepted)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"id":"42","query":"active"}` {
		t.Fatalf("route body = %q, want parameter and query JSON", body)
	}
}

// TestAppReturnsNotFound verifies the response for an unknown path.
func TestAppReturnsNotFound(t *testing.T) {
	app := New(Info{})
	response := executeRequest(app, http.MethodGet, "/missing")

	if response.Code != http.StatusNotFound {
		t.Fatalf("not-found status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if !strings.Contains(response.Body.String(), `"code":"not_found"`) {
		t.Fatalf("not-found body = %q, want not_found code", response.Body.String())
	}
}

// TestAppReturnsMethodNotAllowed verifies that matching paths with the wrong
// method produce a 405 response and preserve the allowed methods.
func TestAppReturnsMethodNotAllowed(t *testing.T) {
	app := New(Info{})
	app.GET("/users", func(Context) error { return nil })
	app.POST("/users", func(Context) error { return nil })

	response := executeRequest(app, http.MethodDelete, "/users")
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method-not-allowed status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get(HeaderRequestID); got == "" {
		t.Fatal("method-not-allowed response should contain a request ID")
	}
	if !strings.Contains(response.Body.String(), `"allow":"GET, POST"`) {
		t.Fatalf("method-not-allowed body = %q, want allowed methods", response.Body.String())
	}
}

// TestAppAppliesMiddleware verifies that middleware added through App.Use is
// applied around the selected route handler.
func TestAppAppliesMiddleware(t *testing.T) {
	app := New(Info{})
	var calls []string
	app.Use(func(next Handler) Handler {
		return func(c Context) error {
			calls = append(calls, "before")
			err := next(c)
			calls = append(calls, "after")
			return err
		}
	})
	app.GET("/middleware", func(Context) error {
		calls = append(calls, "handler")
		return nil
	})

	response := executeRequest(app, http.MethodGet, "/middleware")
	if response.Code != http.StatusOK {
		t.Fatalf("middleware route status = %d, want default 200", response.Code)
	}
	if !reflect.DeepEqual(calls, []string{"before", "handler", "after"}) {
		t.Fatalf("middleware calls = %#v, want before/handler/after", calls)
	}
}

// TestAppWritesHandlerErrors verifies that an error returned before a response
// is written is converted into a JSON HTTP error.
func TestAppWritesHandlerErrors(t *testing.T) {
	app := New(Info{})
	app.GET("/failure", func(Context) error {
		return NewHTTPError(http.StatusBadRequest, "invalid input")
	})

	response := executeRequest(app, http.MethodGet, "/failure")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("handler error status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), `"message":"invalid input"`) {
		t.Fatalf("handler error body = %q, want invalid input", response.Body.String())
	}
}

// TestAppDoesNotOverwriteWrittenResponse verifies that an error returned after
// the handler has written a response does not cause a second response body.
func TestAppDoesNotOverwriteWrittenResponse(t *testing.T) {
	app := New(Info{})
	app.GET("/written", func(c Context) error {
		if err := c.Status(http.StatusAccepted); err != nil {
			return err
		}
		return errors.New("late error")
	})

	response := executeRequest(app, http.MethodGet, "/written")
	if response.Code != http.StatusAccepted {
		t.Fatalf("written response status = %d, want %d", response.Code, http.StatusAccepted)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("written response body = %q, want empty", response.Body.String())
	}
}

// TestJoinAllow verifies the formatting used by 405 errors.
func TestJoinAllow(t *testing.T) {
	if got := joinAllow(nil); got != "" {
		t.Fatalf("joinAllow(nil) = %q, want empty", got)
	}
	if got := joinAllow([]string{http.MethodGet, http.MethodPost}); got != "GET, POST" {
		t.Fatalf("joinAllow() = %q, want GET, POST", got)
	}
}

// TestShutdownWithoutServer verifies that Shutdown is safe before a server is
// started.
func TestShutdownWithoutServer(t *testing.T) {
	if err := New(Info{}).Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() before start returned error: %v", err)
	}
}

// TestListenAndShutdown verifies the controlled listener lifecycle without
// relying on a fixed network port.
func TestListenAndShutdown(t *testing.T) {
	app := New(Info{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen(): %v", err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- app.Listen(listener)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for app.server == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if app.server == nil {
		t.Fatal("Listen() did not initialize the server")
	}

	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("Listen() returned error after shutdown: %v", err)
	}
}

// TestNormalizeServerError verifies that only the standard closed-server
// sentinel is converted to nil.
func TestNormalizeServerError(t *testing.T) {
	if err := normalizeServerError(http.ErrServerClosed); err != nil {
		t.Fatalf("normalizeServerError(ErrServerClosed) = %v, want nil", err)
	}
	want := errors.New("server failed")
	if got := normalizeServerError(want); got != want {
		t.Fatalf("normalizeServerError() = %v, want original error", got)
	}
}

// TestNewServerConfiguration verifies the timeout and handler configuration
// used by ListenAndServe.
func TestNewServerConfiguration(t *testing.T) {
	app := New(Info{})
	server := app.newServer(":8080")

	if server.Addr != ":8080" {
		t.Fatalf("server address = %q, want :8080", server.Addr)
	}
	if server.Handler == nil {
		t.Fatal("server handler should be configured")
	}
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 15*time.Second || server.WriteTimeout != 15*time.Second || server.IdleTimeout != 60*time.Second {
		t.Fatalf("server timeouts = %#v, want configured timeout policy", server)
	}
	if server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, 1<<20)
	}
}

// TestAppUsesStandardHandler verifies that Handler can be passed to the
// standard net/http test server.
func TestAppUsesStandardHandler(t *testing.T) {
	app := New(Info{})
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read health response: %v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("health response = (%d, %q), want 200 and status JSON", response.StatusCode, body)
	}
}

// TestAppLoggerConfiguration verifies that New configures a usable default
// logger without requiring output assertions against the global logger.
func TestAppLoggerConfiguration(t *testing.T) {
	app := New(Info{})
	if app.log == nil || app.log.Enabled(context.Background(), slog.LevelInfo) == false {
		t.Fatal("New() should configure an info-enabled logger")
	}
}

func executeRequest(app *App, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	return response
}
