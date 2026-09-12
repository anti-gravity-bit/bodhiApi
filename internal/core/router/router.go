// Package router implements route registration, matching, and middleware composition.
//
// Routes are stored in registration order and matched with a linear scan so the
// rules are easy to read and test. Static routes take precedence over parameter
// routes. The first matching parameter route is used when no static route hits.
// Trailing slashes are significant on request paths.
package router

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
	corecontext "github.com/anti-gravity-bit/bodhiApi/internal/core/context"
	coreerrors "github.com/anti-gravity-bit/bodhiApi/internal/core/errors"
)

// pathSegment is one element of a registered route pattern.
// A segment beginning with ':' is a named parameter; otherwise it is static.
type pathSegment struct {
	raw         string
	isParameter bool
	name        string
}

// registeredRoute is one method + pattern pair and its handler.
type registeredRoute struct {
	method         string
	pattern        string
	pathSegments   []pathSegment
	isStatic       bool
	requestHandler core.Handler
}

// Match is the result of one route lookup.
//
// Found is true when a handler matches both method and path. Params holds named
// parameter values for that match. Allow lists methods that would match the path
// when Found is false, so callers can produce a 405 with an Allow set.
type Match struct {
	Handler core.Handler
	Params  map[string]string
	Allow   []string
	Found   bool
}

// Router stores registered routes in registration order and applies middleware
// around matched handlers. Static routes take precedence over parameter routes.
type Router struct {
	registeredRoutes []registeredRoute
	middlewareStack  []core.Middleware
}

// New creates an empty router with no routes and no middleware.
func New() *Router { return &Router{} }

// Use appends middleware to the router's execution stack.
// Middleware runs in registration order: the first Use argument is outermost.
func (routerInstance *Router) Use(middleware ...core.Middleware) {
	routerInstance.middlewareStack = append(routerInstance.middlewareStack, middleware...)
}

// GET registers a route for GET requests.
func (routerInstance *Router) GET(path string, handler core.Handler) {
	routerInstance.Handle(http.MethodGet, path, handler)
}

// POST registers a route for POST requests.
func (routerInstance *Router) POST(path string, handler core.Handler) {
	routerInstance.Handle(http.MethodPost, path, handler)
}

// PUT registers a route for PUT requests.
func (routerInstance *Router) PUT(path string, handler core.Handler) {
	routerInstance.Handle(http.MethodPut, path, handler)
}

// PATCH registers a route for PATCH requests.
func (routerInstance *Router) PATCH(path string, handler core.Handler) {
	routerInstance.Handle(http.MethodPatch, path, handler)
}

// DELETE registers a route for DELETE requests.
func (routerInstance *Router) DELETE(path string, handler core.Handler) {
	routerInstance.Handle(http.MethodDelete, path, handler)
}

// Handle registers a route for an HTTP method and path pattern.
// Paths must start with '/'. Methods must be non-empty. Invalid definitions panic
// at registration time so misconfigured routes fail loudly during startup.
func (routerInstance *Router) Handle(method, path string, handler core.Handler) {
	validateRoute(method, path)
	routerInstance.registeredRoutes = append(routerInstance.registeredRoutes, registeredRoute{method: method, pattern: path, pathSegments: parsePathPattern(path), isStatic: !strings.Contains(path, ":"), requestHandler: handler})
}

// validateRoute rejects route definitions that cannot be matched safely.
func validateRoute(method, path string) {
	if path == "" || path[0] != '/' {
		panic("bodhiApi: path must start with /")
	}
	if method == "" {
		panic("bodhiApi: method required")
	}
}

// parsePathPattern splits a registered pattern into segments.
// Leading and trailing slashes are trimmed, so a registered "/users/" is stored
// as the same segments as "/users". Request matching still treats trailing
// slashes as significant via splitRequestPath.
func parsePathPattern(path string) []pathSegment {
	trimmedPath := strings.Trim(path, "/")
	if trimmedPath == "" {
		return nil
	}
	pathParts := strings.Split(trimmedPath, "/")
	parsedSegments := make([]pathSegment, len(pathParts))
	for segmentIndex, pathPart := range pathParts {
		parsedSegments[segmentIndex] = parsePathSegment(pathPart)
	}
	return parsedSegments
}

// parsePathSegment classifies one pattern piece as static or named parameter.
func parsePathSegment(pathPart string) pathSegment {
	parameterName, isParameter := strings.CutPrefix(pathPart, ":")
	if isParameter {
		return pathSegment{raw: pathPart, isParameter: true, name: parameterName}
	}
	return pathSegment{raw: pathPart}
}

// Match resolves a method and path into a handler, parameters, or allowed methods.
//
// Scan order is registration order. A static match returns immediately. A
// parameter match is remembered and used only if no static route wins. Methods
// that match the path but not the request method accumulate in Allow.
func (routerInstance *Router) Match(method, path string) Match {
	pathSegments := splitRequestPath(path)
	var allowedMethods []string
	var parameterRoute *registeredRoute
	var parameterValues map[string]string
	for routeIndex := range routerInstance.registeredRoutes {
		candidateRoute := &routerInstance.registeredRoutes[routeIndex]
		matched, routeParameters := matchSegments(candidateRoute.pathSegments, pathSegments)
		if !matched {
			continue
		}
		allowedMethods = appendUniqueMethod(allowedMethods, candidateRoute.method)
		if candidateRoute.method != method {
			continue
		}
		if candidateRoute.isStatic {
			return Match{Handler: candidateRoute.requestHandler, Params: routeParameters, Found: true}
		}
		if parameterRoute == nil {
			parameterRoute, parameterValues = candidateRoute, routeParameters
		}
	}
	if parameterRoute != nil {
		return Match{Handler: parameterRoute.requestHandler, Params: parameterValues, Found: true}
	}
	if len(allowedMethods) > 0 {
		return Match{Allow: allowedMethods}
	}
	return Match{}
}

// splitRequestPath splits an incoming URL path into comparable segments.
// "/" and "" become a nil segment list so they match a registered root route.
// A trailing slash appends an empty segment, which makes "/users/" different
// from "/users".
func splitRequestPath(path string) []string {
	if path == "" || path == "/" {
		return nil
	}
	hasTrailingSlash := strings.HasSuffix(path, "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if hasTrailingSlash {
		pathParts = append(pathParts, "")
	}
	return pathParts
}

// matchSegments reports whether a pattern and a request path have the same
// length and the same static pieces. Parameter names are filled from the
// corresponding request segments.
func matchSegments(patternSegments []pathSegment, pathSegments []string) (bool, map[string]string) {
	if len(patternSegments) != len(pathSegments) {
		return false, nil
	}
	routeParameters := make(map[string]string)
	for segmentIndex, expectedSegment := range patternSegments {
		if expectedSegment.isParameter {
			routeParameters[expectedSegment.name] = pathSegments[segmentIndex]
			continue
		}
		if expectedSegment.raw != pathSegments[segmentIndex] {
			return false, nil
		}
	}
	return true, routeParameters
}

// appendUniqueMethod appends method if it is not already present, preserving order.
func appendUniqueMethod(methods []string, method string) []string {
	if slices.Contains(methods, method) {
		return methods
	}
	return append(methods, method)
}

// Apply wraps a handler so middleware runs in registration order.
// Wrapping starts from the last registered middleware so the first registered
// middleware becomes the outermost call.
func (routerInstance *Router) Apply(handler core.Handler) core.Handler {
	for middlewareIndex := len(routerInstance.middlewareStack) - 1; middlewareIndex >= 0; middlewareIndex-- {
		handler = routerInstance.middlewareStack[middlewareIndex](handler)
	}
	return handler
}

// Handler adapts the router to net/http. It is useful for standalone router tests
// and for applications that do not need the App lifecycle.
//
// Unmatched paths become 404 JSON errors. Paths that match another method become
// 405 JSON errors. Middleware still runs for those fallbacks. A handler error is
// written only when the handler has not already written a response.
func (routerInstance *Router) Handler() http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		matchResult := routerInstance.Match(httpRequest.Method, httpRequest.URL.Path)
		matchedHandler := matchResult.Handler
		if !matchResult.Found {
			if len(matchResult.Allow) > 0 {
				allowedMethods := strings.Join(matchResult.Allow, ", ")
				matchedHandler = func(core.Context) error { return coreerrors.MethodNotAllowed(allowedMethods) }
			} else {
				matchedHandler = func(core.Context) error { return coreerrors.NotFound("not found") }
			}
		}
		requestContext := corecontext.New(responseWriter, httpRequest, matchResult.Params)
		if handlerError := routerInstance.Apply(matchedHandler)(requestContext); handlerError != nil && !requestContext.Wrote() {
			httpError := coreerrors.AsHTTPError(handlerError)
			responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
			responseWriter.WriteHeader(httpError.Status)
			_ = json.NewEncoder(responseWriter).Encode(httpError)
		}
	})
}
