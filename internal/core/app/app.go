// Package app implements the application lifecycle and HTTP request pipeline.
//
// App owns the router, default middleware, structured logger, and net/http
// server. Incoming requests are matched, wrapped in middleware, executed against
// a request-scoped context, and converted into JSON error responses when a
// handler returns an error without writing a body.
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
	corecontext "github.com/anti-gravity-bit/bodhiApi/internal/core/context"
	coreerrors "github.com/anti-gravity-bit/bodhiApi/internal/core/errors"
	coremiddleware "github.com/anti-gravity-bit/bodhiApi/internal/core/middleware"
	"github.com/anti-gravity-bit/bodhiApi/internal/core/router"
)

// Info identifies an application in logs and process metadata.
// Values are not interpreted by the router.
type Info struct {
	// Title is a human-readable application name, for example "Users API".
	Title string
	// Version is an informational version string, for example "0.0.0".
	Version string
}

// App owns routing, middleware, logging, and HTTP server lifecycle.
//
// A zero App is not useful; construct one with New so default middleware and
// the health route are installed. Server fields are guarded by serverMutex
// because Listen/ListenAndServe and Shutdown may run on different goroutines.
type App struct {
	// Info is the identity recorded in the listening log line.
	Info Info

	applicationRouter *router.Router
	applicationLogger *slog.Logger
	httpServer        *http.Server
	serverMutex       sync.RWMutex
}

// New creates an application with default middleware and GET /health.
//
// Default middleware, in order, is panic recovery, request-ID assignment, and
// structured request logging. The health handler returns {"status":"ok"}.
func New(info Info) *App {
	applicationRouter := router.New()
	applicationLogger := slog.Default()
	applicationRouter.Use(recoverMiddleware(), requestIDMiddleware(), loggerMiddleware(applicationLogger))
	application := &App{Info: info, applicationRouter: applicationRouter, applicationLogger: applicationLogger}
	applicationRouter.GET("/health", func(requestContext core.Context) error {
		return requestContext.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	return application
}

// recoverMiddleware returns the default panic-recovery middleware.
func recoverMiddleware() core.Middleware { return coremiddleware.Recover }

// requestIDMiddleware returns the default request-ID middleware.
func requestIDMiddleware() core.Middleware { return coremiddleware.RequestID }

// loggerMiddleware returns structured request logging using the given logger.
func loggerMiddleware(logger *slog.Logger) core.Middleware { return coremiddleware.Logger(logger) }

// Router returns the application router. Routes registered here participate in
// the same middleware stack as routes registered through App method helpers.
func (application *App) Router() *router.Router { return application.applicationRouter }

// GET registers a handler for HTTP GET requests on path.
func (application *App) GET(path string, handler core.Handler) {
	application.applicationRouter.GET(path, handler)
}

// POST registers a handler for HTTP POST requests on path.
func (application *App) POST(path string, handler core.Handler) {
	application.applicationRouter.POST(path, handler)
}

// PUT registers a handler for HTTP PUT requests on path.
func (application *App) PUT(path string, handler core.Handler) {
	application.applicationRouter.PUT(path, handler)
}

// PATCH registers a handler for HTTP PATCH requests on path.
func (application *App) PATCH(path string, handler core.Handler) {
	application.applicationRouter.PATCH(path, handler)
}

// DELETE registers a handler for HTTP DELETE requests on path.
func (application *App) DELETE(path string, handler core.Handler) {
	application.applicationRouter.DELETE(path, handler)
}

// Use appends middleware to the application stack. Middleware runs in
// registration order around every request, including unmatched routes.
func (application *App) Use(middleware ...core.Middleware) {
	application.applicationRouter.Use(middleware...)
}

// Handler returns a net/http handler that dispatches through the application
// router. It is safe to use with httptest and with a custom http.Server.
func (application *App) Handler() http.Handler { return http.HandlerFunc(application.serve) }

// serve matches the request, applies middleware, and writes a JSON error when
// the handler fails without already writing a response.
func (application *App) serve(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	matchResult := application.applicationRouter.Match(httpRequest.Method, httpRequest.URL.Path)
	matchedHandler := matchResult.Handler
	if !matchResult.Found {
		if len(matchResult.Allow) > 0 {
			allowedMethods := strings.Join(matchResult.Allow, ", ")
			matchedHandler = func(core.Context) error { return coreerrors.MethodNotAllowed(allowedMethods) }
		} else {
			matchedHandler = func(core.Context) error { return coreerrors.NotFound("not found") }
		}
	}
	matchedHandler = application.applicationRouter.Apply(matchedHandler)
	requestContext := corecontext.New(responseWriter, httpRequest, matchResult.Params)
	if handlerError := matchedHandler(requestContext); handlerError != nil && !requestContext.Wrote() {
		writeError(responseWriter, handlerError)
	}
}

// writeError converts sourceError into the public JSON error contract and writes
// it. Encoding errors are ignored because the status has already been sent.
func writeError(responseWriter http.ResponseWriter, sourceError error) {
	httpError := coreerrors.AsHTTPError(sourceError)
	responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	responseWriter.WriteHeader(httpError.Status)
	_ = json.NewEncoder(responseWriter).Encode(httpError)
}

// ListenAndServe starts an HTTP server on address and blocks until the server
// stops. http.ErrServerClosed is normalized to nil so Shutdown is not an error.
func (application *App) ListenAndServe(address string) error {
	configuredServer := newServer(application, address)
	application.setServer(configuredServer)
	application.applicationLogger.Info("listening", slog.String("addr", address), slog.String("title", application.Info.Title))
	return normalizeServerError(configuredServer.ListenAndServe())
}

// Listen serves HTTP on an existing listener. Timeouts other than
// ReadHeaderTimeout follow net/http.Server defaults because the caller owns
// the listener. http.ErrServerClosed is normalized to nil.
func (application *App) Listen(networkListener net.Listener) error {
	configuredServer := &http.Server{Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second}
	application.setServer(configuredServer)
	return normalizeServerError(configuredServer.Serve(networkListener))
}

// Shutdown gracefully stops the HTTP server. It is a no-op when no server has
// been started, so callers can shut down during tests or failed boot.
func (application *App) Shutdown(shutdownContext context.Context) error {
	application.serverMutex.RLock()
	configuredServer := application.httpServer
	application.serverMutex.RUnlock()
	if configuredServer == nil {
		return nil
	}
	return configuredServer.Shutdown(shutdownContext)
}

// newServer builds the production http.Server used by ListenAndServe.
// Timeouts and header limits are conservative defaults for JSON APIs.
func newServer(application *App, address string) *http.Server {
	return &http.Server{Addr: address, Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}

// setServer stores the running server so Shutdown can find it.
func (application *App) setServer(configuredServer *http.Server) {
	application.serverMutex.Lock()
	application.httpServer = configuredServer
	application.serverMutex.Unlock()
}

// normalizeServerError treats a graceful close as success and preserves every
// other listen/serve error for the caller.
func normalizeServerError(err error) error {
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
