package bodhiApi

import (
	"encoding/json"
	"net/http"
)

// Context is the per-request handle shared by handlers and middleware.
//
// It provides access to the request, response writer, route values, request
// inputs, response helpers, and a small request-scoped key/value store. The
// interface is intentionally small so handlers do not need to know about the
// router's internal implementation.
type Context interface {
	Request() *http.Request
	ResponseWriter() http.ResponseWriter

	Param(name string) string
	Query(name string) string
	Header(name string) string

	JSON(status int, v any) error
	Status(code int) error

	// Set and Get store values for the lifetime of one request. This is useful
	// for values such as a request ID that middleware prepares for handlers.
	Set(key string, value any)
	Get(key string) (any, bool)
}

// bodhiContext is the concrete Context used internally by the package.
//
// The fields are grouped by responsibility:
//   - w and r connect the context to the HTTP exchange.
//   - params contains values captured from parameterized routes.
//   - store contains request-scoped values shared by middleware and handlers.
//   - wrote records whether this context has attempted to write a response.
type bodhiContext struct {
	w      http.ResponseWriter
	r      *http.Request
	params map[string]string
	store  map[string]any
	wrote  bool
}

// newContext creates the request context used by a matched route.
//
// A nil parameter map is normalized to an empty map so Param can be called
// safely for routes that do not contain parameters.
func newContext(w http.ResponseWriter, r *http.Request, params map[string]string) *bodhiContext {
	if params == nil {
		params = map[string]string{}
	}

	return &bodhiContext{
		w:      w,
		r:      r,
		params: params,
		store:  map[string]any{},
	}
}

// Request returns the incoming HTTP request.
func (c *bodhiContext) Request() *http.Request {
	return c.r
}

// ResponseWriter returns the writer used for the outgoing HTTP response.
func (c *bodhiContext) ResponseWriter() http.ResponseWriter {
	return c.w
}

// Param returns a route parameter by name. Missing parameters return the empty
// string, matching map lookup behavior for the internal parameter map.
func (c *bodhiContext) Param(name string) string {
	return c.params[name]
}

// Query returns the first query-string value for name. Missing values return
// the empty string, as provided by url.Values.Get.
func (c *bodhiContext) Query(name string) string {
	return c.r.URL.Query().Get(name)
}

// Header returns the request header value for name.
func (c *bodhiContext) Header(name string) string {
	return c.r.Header.Get(name)
}

// JSON writes a JSON response with the supplied HTTP status code.
//
// The content type is set before the status is written. The JSON encoder then
// writes the value and appends a newline, which is standard Encoder behavior.
func (c *bodhiContext) JSON(status int, value any) error {
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(status)
	c.wrote = true

	return json.NewEncoder(c.w).Encode(value)
}

// Status writes only the supplied HTTP status code and marks the response as
// written. It does not write a response body.
func (c *bodhiContext) Status(code int) error {
	c.w.WriteHeader(code)
	c.wrote = true
	return nil
}

// Set stores a request-scoped value under key.
func (c *bodhiContext) Set(key string, value any) {
	c.store[key] = value
}

// Get retrieves a request-scoped value. The boolean reports whether key was
// present, including when the stored value itself is nil.
func (c *bodhiContext) Get(key string) (any, bool) {
	value, ok := c.store[key]
	return value, ok
}
