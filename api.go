// Package bodhiApi is a small, readable, zero-dependency foundation for building
// HTTP JSON APIs on top of the Go standard library.
//
// The root package is the stable public facade. Applications should import this
// package and ignore internal layout. Implementation details live in
// internal/core so routing, request context, errors, middleware, and server
// lifecycle can evolve independently without changing the import path:
//
//	github.com/anti-gravity-bit/bodhiApi
//
// Typical usage:
//
//	application := bodhiApi.New(bodhiApi.Info{Title: "Users API", Version: "0.0.0"})
//	application.GET("/users/:id", func(requestContext bodhiApi.Context) error {
//		return requestContext.JSON(http.StatusOK, map[string]string{
//			"id": requestContext.Param("id"),
//		})
//	})
//	_ = application.ListenAndServe(":8080")
package bodhiApi

import (
	"log/slog"
	"time"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
	coreapp "github.com/anti-gravity-bit/bodhiApi/internal/core/app"
	corecontext "github.com/anti-gravity-bit/bodhiApi/internal/core/context"
	coreerrors "github.com/anti-gravity-bit/bodhiApi/internal/core/errors"
	coremiddleware "github.com/anti-gravity-bit/bodhiApi/internal/core/middleware"
	corerouter "github.com/anti-gravity-bit/bodhiApi/internal/core/router"
)

// Info identifies an application in logs and future metadata surfaces.
// Title and Version are informational; they do not affect routing.
type Info = coreapp.Info

// App is the main application object: router, middleware stack, and HTTP server
// lifecycle (ListenAndServe, Listen, Shutdown).
type App = coreapp.App

// Context is the request-scoped API passed to handlers and middleware.
// Each request receives an isolated implementation; values stored with Set are
// not visible to other requests.
type Context = core.Context

// Handler processes one HTTP request and may return an error. A returned error
// is converted into a JSON HTTP error when the handler has not already written
// a response.
type Handler = core.Handler

// Middleware wraps a Handler. Middleware registered with Use runs in
// registration order around every request, including 404 and 405 responses.
type Middleware = core.Middleware

// HTTPError is the JSON error contract returned to API clients. Unknown
// implementation errors are converted to a generic internal error so internal
// details are not leaked.
type HTTPError = coreerrors.HTTPError

// Router stores routes and middleware independently of App server lifecycle.
// Use it when you need matching or a net/http handler without ListenAndServe.
type Router = corerouter.Router

// Group is a prefixed subset of routes sharing a Router and optional middleware.
type Group = corerouter.Group

// RequestContext is the concrete request-scoped context implementation.
// Most applications should depend on the Context interface instead.
type RequestContext = corecontext.RequestContext

// HeaderRequestID is the canonical request-correlation header used by RequestID
// middleware. Incoming values are reused; otherwise a new identifier is generated.
const HeaderRequestID = coremiddleware.HeaderRequestID

// New constructs an App with default middleware (panic recovery, request IDs,
// structured logs) and a built-in GET /health route.
func New(info Info) *App { return coreapp.New(info) }

// NewRouter constructs an empty Router with no middleware and no routes.
func NewRouter() *Router { return corerouter.New() }

// NewHTTPError constructs an HTTPError with the given status and message.
// The Code field is left empty; use NotFound or MethodNotAllowed for coded errors.
func NewHTTPError(status int, message string) *HTTPError { return coreerrors.New(status, message) }

// NotFound returns a 404 HTTPError. An empty message becomes "Not Found".
func NotFound(message string) *HTTPError { return coreerrors.NotFound(message) }

// MethodNotAllowed returns a 405 HTTPError whose details include the allowed
// methods string, typically a comma-separated list from the router.
func MethodNotAllowed(allow string) *HTTPError { return coreerrors.MethodNotAllowed(allow) }

// AsHTTPError preserves HTTPError values (including wrapped ones) and converts
// every other error into a generic 500 response. A nil input returns nil.
func AsHTTPError(err error) *HTTPError { return coreerrors.AsHTTPError(err) }

// Recover converts panics from the wrapped handler into an internal-server-error
// HTTPError. It is the public form of the default recovery middleware.
func Recover(nextHandler Handler) Handler { return coremiddleware.Recover(nextHandler) }

// RequestID reuses X-Request-ID when present, otherwise generates one, stores it
// on the request context, and writes it on the response.
func RequestID(nextHandler Handler) Handler { return coremiddleware.RequestID(nextHandler) }

// Logger returns middleware that emits one structured slog record per request.
// A nil logger uses slog.Default().
func Logger(logger *slog.Logger) Middleware { return coremiddleware.Logger(logger) }

// MaxBytes limits the request body to maxBytes using http.MaxBytesReader.
func MaxBytes(maxBytes int64) Middleware { return coremiddleware.MaxBytes(maxBytes) }

// Timeout sets a deadline on the request context for downstream handlers.
func Timeout(duration time.Duration) Middleware { return coremiddleware.Timeout(duration) }

// Compile-time assertion: the concrete request context implements Context.
var _ Context = (*RequestContext)(nil)
