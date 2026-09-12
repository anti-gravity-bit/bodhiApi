package bodhiApi

import (
	"net/http"
	"slices"
	"strings"
)

// Handler is the function called when a route matches an incoming request.
// Returning an error lets the caller decide how to turn handler failures into
// HTTP responses.
type Handler func(c Context) error

// Middleware wraps a handler. Middleware is applied in registration order:
// the first middleware registered with Use is the outermost wrapper.
type Middleware func(Handler) Handler

// segment is one piece of a route pattern.
//
// For example, /users/:id is stored as two segments:
//   - "users", a literal segment
//   - ":id", a parameter segment whose name is "id"
type segment struct {
	raw     string
	isParam bool
	name    string
}

// route stores the information needed to match and execute one registered
// route. Routes are kept in registration order.
type route struct {
	method  string
	pattern string
	segs    []segment
	static  bool
	handler Handler
}

// Router uses a simple slice and linear scan.
//
// This keeps registration and matching easy to follow. During matching, exact
// static routes are preferred immediately, while parameter routes are kept as
// a fallback until all routes have been checked.
type Router struct {
	routes []route
	stack  []Middleware
}

// NewRouter creates an empty router.
func NewRouter() *Router {
	return &Router{}
}

// Use adds middleware to the router's middleware stack.
func (r *Router) Use(mw ...Middleware) {
	r.stack = append(r.stack, mw...)
}

// GET registers a route for GET requests.
func (r *Router) GET(path string, h Handler) {
	r.Handle(http.MethodGet, path, h)
}

// POST registers a route for POST requests.
func (r *Router) POST(path string, h Handler) {
	r.Handle(http.MethodPost, path, h)
}

// PUT registers a route for PUT requests.
func (r *Router) PUT(path string, h Handler) {
	r.Handle(http.MethodPut, path, h)
}

// PATCH registers a route for PATCH requests.
func (r *Router) PATCH(path string, h Handler) {
	r.Handle(http.MethodPatch, path, h)
}

// DELETE registers a route for DELETE requests.
func (r *Router) DELETE(path string, h Handler) {
	r.Handle(http.MethodDelete, path, h)
}

// Handle registers a route for the supplied HTTP method and path.
//
// Registration fails fast for malformed paths and missing methods. Keeping
// these checks here means every route, including routes registered through a
// convenience method such as GET, follows the same rules.
func (r *Router) Handle(method, path string, h Handler) {
	validateRoute(method, path)

	r.routes = append(r.routes, route{
		method:  method,
		pattern: path,
		segs:    parsePattern(path),
		static:  !strings.Contains(path, ":"),
		handler: h,
	})
}

// validateRoute checks the two pieces of route input that are required for
// matching: a non-empty method and an absolute path beginning with "/".
func validateRoute(method, path string) {
	if path == "" || path[0] != '/' {
		panic("bodhiApi: path must start with /")
	}

	if method == "" {
		panic("bodhiApi: method required")
	}
}

// parsePattern converts a route pattern into segments.
//
// Slashes at the beginning and end are ignored for pattern matching. A
// segment beginning with ":" is a parameter; all other segments are matched
// literally.
func parsePattern(path string) []segment {
	trimmedPath := strings.Trim(path, "/")
	if trimmedPath == "" {
		return nil
	}

	parts := strings.Split(trimmedPath, "/")
	segments := make([]segment, len(parts))

	for i, part := range parts {
		segments[i] = patternSegment(part)
	}

	return segments
}

// patternSegment converts one pattern part into either a literal or parameter
// segment.
func patternSegment(part string) segment {
	parameterName, isParameter := strings.CutPrefix(part, ":")
	if isParameter {
		return segment{
			raw:     part,
			isParam: true,
			name:    parameterName,
		}
	}

	return segment{raw: part}
}

type matchResult struct {
	handler Handler
	params  map[string]string
	allow   []string
	found   bool
}

// match finds the handler for method and path.
//
// Matching has three possible outcomes:
//  1. A matching handler is returned with found=true.
//  2. No handler matches the method, but allow contains methods registered for
//     the same path. This is useful for producing a 405 response.
//  3. Nothing matches, so the zero value is returned.
func (r *Router) match(method, path string) matchResult {
	pathSegments := splitPath(path)

	var allowedMethods []string
	var parameterRoute *route
	var parameterValues map[string]string

	for i := range r.routes {
		candidate := &r.routes[i]
		matches, params := matchSegments(candidate.segs, pathSegments)
		if !matches {
			continue
		}

		allowedMethods = appendUnique(allowedMethods, candidate.method)
		if candidate.method != method {
			continue
		}

		// A static route is the strongest match. Return it immediately, even if
		// a parameter route appeared earlier in the registration list.
		if candidate.static {
			return matchResult{
				handler: candidate.handler,
				params:  params,
				found:   true,
			}
		}

		// Keep the first parameter route as the fallback for this method. The
		// loop continues so a later static route can still take precedence.
		if parameterRoute == nil {
			parameterRoute = candidate
			parameterValues = params
		}
	}

	if parameterRoute != nil {
		return matchResult{
			handler: parameterRoute.handler,
			params:  parameterValues,
			found:   true,
		}
	}

	if len(allowedMethods) > 0 {
		return matchResult{allow: allowedMethods}
	}

	return matchResult{}
}

// splitPath turns a request path into the same segment shape used by route
// patterns. A trailing slash is preserved as an empty final segment, so /x
// and /x/ remain distinct paths.
func splitPath(path string) []string {
	if path == "" || path == "/" {
		return nil
	}

	hasTrailingSlash := strings.HasSuffix(path, "/")
	trimmedPath := strings.Trim(path, "/")
	parts := strings.Split(trimmedPath, "/")

	if hasTrailingSlash {
		parts = append(parts, "")
	}

	return parts
}

// matchSegments compares a parsed route pattern with a parsed request path.
// Literal segments must be equal. Parameter segments accept any value and add
// that value to the returned parameter map.
func matchSegments(pattern []segment, path []string) (bool, map[string]string) {
	if len(pattern) != len(path) {
		return false, nil
	}

	params := make(map[string]string)
	for i, expected := range pattern {
		if expected.isParam {
			params[expected.name] = path[i]
			continue
		}

		if expected.raw != path[i] {
			return false, nil
		}
	}

	return true, params
}

func appendUnique(dst []string, value string) []string {
	if slices.Contains(dst, value) {
		return dst
	}

	return append(dst, value)
}

// apply wraps a handler with all registered middleware. Wrapping happens in
// reverse order so execution happens in registration order:
//
//	Use(first)
//	Use(second)
//	handler
//
// becomes first(second(handler)).
func (r *Router) apply(handler Handler) Handler {
	for i := len(r.stack) - 1; i >= 0; i-- {
		handler = r.stack[i](handler)
	}

	return handler
}
