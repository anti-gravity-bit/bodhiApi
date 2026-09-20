// Package core contains the implementation contracts shared by bodhiApi's public
// facade and the internal packages that implement it.
//
// This package is internal by design. Applications should import the module root
// (github.com/anti-gravity-bit/bodhiApi). Maintainers can evolve each
// implementation area without exposing internals or creating import cycles:
// internal packages must not import the public root package.
package core

import (
	"net/http"
)

// Context is the request-scoped API exposed to handlers and middleware.
//
// Implementations must keep stored values isolated to one HTTP request. Methods
// that write a response (JSON, Status) are responsible for recording that a
// response has been written so the application does not overwrite it when a
// later error is returned. A second write must not call WriteHeader again.
type Context interface {
	// Request returns the original incoming net/http request.
	Request() *http.Request
	// ResponseWriter returns the writer for the outgoing response.
	ResponseWriter() http.ResponseWriter
	// Param returns a named route parameter such as :id, or "" when missing.
	Param(name string) string
	// Query returns the first query-string value for name, or "" when missing.
	Query(name string) string
	// Header returns the incoming request header value for name, or "".
	Header(name string) string
	// JSON writes a JSON body with the given HTTP status code.
	JSON(status int, value any) error
	// Status writes only an HTTP status code and an empty body.
	Status(code int) error
	// Set stores a request-scoped value for later middleware or handlers.
	Set(key string, value any)
	// Get retrieves a request-scoped value and reports whether it exists.
	Get(key string) (any, bool)
}

// Handler processes one request and returns an optional application error.
// Returning an error lets the application produce a consistent JSON HTTP
// response when the handler has not already written one.
type Handler func(Context) error

// Middleware wraps a Handler. Middleware is applied in registration order:
// the first registered middleware is the outermost wrapper.
type Middleware func(Handler) Handler
