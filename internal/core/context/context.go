// Package context implements the request-scoped context used by handlers and
// middleware.
//
// RequestContext binds one incoming *http.Request to one http.ResponseWriter,
// exposes route parameters, query values, and headers, and records whether a
// response has already been written so the application will not overwrite it.
package context

import (
	"encoding/json"
	"net/http"
)

// Context describes the request API implemented by RequestContext.
// It matches the public core.Context contract so handlers can depend on either
// the interface or the concrete type.
type Context interface {
	Request() *http.Request
	ResponseWriter() http.ResponseWriter
	Param(name string) string
	Query(name string) string
	Header(name string) string
	JSON(status int, value any) error
	Status(code int) error
	Set(key string, value any)
	Get(key string) (any, bool)
}

// RequestContext owns the state of one HTTP request and response exchange.
//
// It is not safe for concurrent use by multiple goroutines unless those
// goroutines only read after the handler has finished writing. Values stored
// with Set live only for this request.
type RequestContext struct {
	responseWriter  http.ResponseWriter
	httpRequest     *http.Request
	routeParameters map[string]string
	requestValues   map[string]any
	responseWritten bool
}

// Compile-time check: RequestContext implements this package's Context interface.
var _ Context = (*RequestContext)(nil)

// New creates a request context. A nil parameter map is normalized to an empty
// map so Param never panics. Request-scoped storage starts empty.
func New(responseWriter http.ResponseWriter, httpRequest *http.Request, routeParameters map[string]string) *RequestContext {
	if routeParameters == nil {
		routeParameters = map[string]string{}
	}
	return &RequestContext{responseWriter: responseWriter, httpRequest: httpRequest, routeParameters: routeParameters, requestValues: map[string]any{}}
}

// Request returns the original incoming HTTP request.
func (requestContext *RequestContext) Request() *http.Request { return requestContext.httpRequest }

// ResponseWriter returns the writer used for the outgoing response.
func (requestContext *RequestContext) ResponseWriter() http.ResponseWriter {
	return requestContext.responseWriter
}

// Param returns a named route parameter or an empty string when it is missing.
func (requestContext *RequestContext) Param(name string) string {
	return requestContext.routeParameters[name]
}

// Query returns the first query-string value for name, or "" when absent.
func (requestContext *RequestContext) Query(name string) string {
	return requestContext.httpRequest.URL.Query().Get(name)
}

// Header returns the incoming request header value for name, or "".
// Lookup uses net/http canonical header keys.
func (requestContext *RequestContext) Header(name string) string {
	return requestContext.httpRequest.Header.Get(name)
}

// JSON writes a JSON response and records that the response was written.
//
// Content-Type is set before the status so clients always see a JSON type on
// success. The written flag is set before encoding so a later application error
// converter will not replace a partially sent body. If encoding fails, the
// caller receives that error after headers and status have already been sent.
func (requestContext *RequestContext) JSON(status int, value any) error {
	requestContext.responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	requestContext.responseWriter.WriteHeader(status)
	requestContext.responseWritten = true
	return json.NewEncoder(requestContext.responseWriter).Encode(value)
}

// Status writes only an HTTP status code and records that the response was written.
func (requestContext *RequestContext) Status(statusCode int) error {
	requestContext.responseWriter.WriteHeader(statusCode)
	requestContext.responseWritten = true
	return nil
}

// Set stores a request-scoped value, replacing any previous value for key.
func (requestContext *RequestContext) Set(key string, value any) {
	requestContext.requestValues[key] = value
}

// Get retrieves a request-scoped value and reports whether it exists.
// A stored nil is found=true, value=nil. A missing key is found=false.
func (requestContext *RequestContext) Get(key string) (any, bool) {
	value, found := requestContext.requestValues[key]
	return value, found
}

// Wrote reports whether a response-writing method has been called.
// The application uses this to avoid overwriting an already-sent response when
// a handler returns a late error.
func (requestContext *RequestContext) Wrote() bool { return requestContext.responseWritten }
