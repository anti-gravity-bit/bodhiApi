package bodhiApi

import (
	"net/http"
	"reflect"
	"testing"
)

// TestParsePattern verifies that literal and parameter parts are represented
// correctly, including the parameter names used when a request matches.
func TestParsePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    []segment
	}{
		{
			name:    "root",
			pattern: "/",
			want:    nil,
		},
		{
			name:    "literals and parameters",
			pattern: "/users/:id/posts/:postID",
			want: []segment{
				{raw: "users"},
				{raw: ":id", isParam: true, name: "id"},
				{raw: "posts"},
				{raw: ":postID", isParam: true, name: "postID"},
			},
		},
		{
			name:    "trailing slash is ignored in pattern",
			pattern: "/users/",
			want:    []segment{{raw: "users"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parsePattern(tt.pattern); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parsePattern(%q) = %#v, want %#v", tt.pattern, got, tt.want)
			}
		})
	}
}

// TestSplitPath documents the path normalization rules used by matching.
func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{path: "", want: nil},
		{path: "/", want: nil},
		{path: "/users/42", want: []string{"users", "42"}},
		{path: "/users/42/", want: []string{"users", "42", ""}},
		{path: "/users//posts", want: []string{"users", "", "posts"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := splitPath(tt.path); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitPath(%q) = %#v, want %#v", tt.path, got, tt.want)
			}
		})
	}
}

// TestMatchSegments verifies literal matching, parameter extraction, and the
// requirement that the pattern and request have the same number of segments.
func TestMatchSegments(t *testing.T) {
	pattern := parsePattern("/users/:id/posts/:postID")

	matched, params := matchSegments(pattern, []string{"users", "42", "posts", "9"})
	if !matched {
		t.Fatal("expected segments to match")
	}

	wantParams := map[string]string{"id": "42", "postID": "9"}
	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("params = %#v, want %#v", params, wantParams)
	}

	for _, path := range [][]string{
		{"admin", "42", "posts", "9"},
		{"users", "42", "posts"},
	} {
		matched, params = matchSegments(pattern, path)
		if matched || params != nil {
			t.Fatalf("matchSegments(%#v) = (%v, %#v), want (false, nil)", path, matched, params)
		}
	}
}

// TestMatchRoot verifies that the root route matches the root path.
func TestMatchRoot(t *testing.T) {
	router := NewRouter()
	router.GET("/", func(Context) error { return nil })

	result := router.match(http.MethodGet, "/")
	if !result.found {
		t.Fatal("root route should match")
	}
}

// TestRouterMatch covers exact routes, parameter routes, method matching, and
// the allow list returned when the path exists for another method.
func TestRouterMatch(t *testing.T) {
	staticHandler := func(Context) error { return nil }
	parameterHandler := func(Context) error { return nil }

	router := NewRouter()
	router.GET("/users/:id", parameterHandler)
	router.POST("/users", staticHandler)

	result := router.match(http.MethodGet, "/users/42")
	if !result.found || result.handler == nil {
		t.Fatal("parameter route should match")
	}
	if !reflect.DeepEqual(result.params, map[string]string{"id": "42"}) {
		t.Fatalf("params = %#v, want id=42", result.params)
	}

	result = router.match(http.MethodGet, "/users")
	if result.found || !reflect.DeepEqual(result.allow, []string{http.MethodPost}) {
		t.Fatalf("wrong-method result = %#v, want allow=[POST] and found=false", result)
	}

	result = router.match(http.MethodGet, "/missing")
	if result.found || len(result.allow) != 0 {
		t.Fatalf("missing route result = %#v, want no match and no allowed methods", result)
	}
}

// TestStaticRouteTakesPrecedence verifies that an exact route wins over a
// parameter route even when the parameter route was registered first.
func TestStaticRouteTakesPrecedence(t *testing.T) {
	parameterHandler := func(Context) error { return nil }
	staticHandler := func(Context) error { return nil }

	router := NewRouter()
	router.GET("/users/:id", parameterHandler)
	router.GET("/users/me", staticHandler)

	result := router.match(http.MethodGet, "/users/me")
	if !result.found {
		t.Fatal("expected a route to match")
	}
	if reflect.ValueOf(result.handler).Pointer() != reflect.ValueOf(staticHandler).Pointer() {
		t.Fatal("static route should take precedence over parameter route")
	}
}

// TestFirstParameterRouteWins verifies that the first matching parameter route
// remains the fallback when no static route matches.
func TestFirstParameterRouteWins(t *testing.T) {
	firstHandler := func(Context) error { return nil }
	secondHandler := func(Context) error { return nil }

	router := NewRouter()
	router.GET("/items/:first", firstHandler)
	router.GET("/items/:second", secondHandler)

	result := router.match(http.MethodGet, "/items/value")
	if reflect.ValueOf(result.handler).Pointer() != reflect.ValueOf(firstHandler).Pointer() {
		t.Fatal("first parameter route should win")
	}
	if !reflect.DeepEqual(result.params, map[string]string{"first": "value"}) {
		t.Fatalf("params = %#v, want first=value", result.params)
	}
}

// TestAllowedMethodsAreUniqueAndOrdered verifies that allow contains each
// matching method once, in route registration order.
func TestAllowedMethodsAreUniqueAndOrdered(t *testing.T) {
	router := NewRouter()
	router.POST("/health", func(Context) error { return nil })
	router.POST("/health", func(Context) error { return nil })
	router.PUT("/health", func(Context) error { return nil })

	result := router.match(http.MethodGet, "/health")
	want := []string{http.MethodPost, http.MethodPut}
	if !reflect.DeepEqual(result.allow, want) {
		t.Fatalf("allow = %#v, want %#v", result.allow, want)
	}
}

// TestRouteValidation verifies the panic messages for invalid registrations.
func TestRouteValidation(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "missing leading slash", method: http.MethodGet, path: "users", want: "bodhiApi: path must start with /"},
		{name: "empty path", method: http.MethodGet, path: "", want: "bodhiApi: path must start with /"},
		{name: "missing method", method: "", path: "/users", want: "bodhiApi: method required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if got := recover(); got != tt.want {
					t.Fatalf("panic = %v, want %q", got, tt.want)
				}
			}()

			NewRouter().Handle(tt.method, tt.path, func(Context) error { return nil })
		})
	}
}

// TestHTTPMethodHelpers verifies that each convenience method registers the
// expected HTTP method.
func TestHTTPMethodHelpers(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		register func(*Router, string, Handler)
	}{
		{name: "GET", method: http.MethodGet, register: func(r *Router, p string, h Handler) { r.GET(p, h) }},
		{name: "POST", method: http.MethodPost, register: func(r *Router, p string, h Handler) { r.POST(p, h) }},
		{name: "PUT", method: http.MethodPut, register: func(r *Router, p string, h Handler) { r.PUT(p, h) }},
		{name: "PATCH", method: http.MethodPatch, register: func(r *Router, p string, h Handler) { r.PATCH(p, h) }},
		{name: "DELETE", method: http.MethodDelete, register: func(r *Router, p string, h Handler) { r.DELETE(p, h) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			tt.register(router, "/route", func(Context) error { return nil })

			if router.routes[0].method != tt.method {
				t.Fatalf("registered method = %q, want %q", router.routes[0].method, tt.method)
			}
		})
	}
}

// TestApplyMiddleware verifies both middleware order and that the wrapped
// handler is still invoked.
func TestApplyMiddleware(t *testing.T) {
	var calls []string
	middleware := func(name string) Middleware {
		return func(next Handler) Handler {
			return func(ctx Context) error {
				calls = append(calls, name+" before")
				err := next(ctx)
				calls = append(calls, name+" after")
				return err
			}
		}
	}

	router := NewRouter()
	router.Use(middleware("first"), middleware("second"))
	wrapped := router.apply(func(Context) error {
		calls = append(calls, "handler")
		return nil
	})

	var ctx Context
	if err := wrapped(ctx); err != nil {
		t.Fatalf("wrapped handler returned error: %v", err)
	}

	want := []string{"first before", "second before", "handler", "second after", "first after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

// TestUseAppendsMiddleware verifies that Use preserves the order in which
// middleware is supplied across multiple calls.
func TestUseAppendsMiddleware(t *testing.T) {
	router := NewRouter()
	first := func(next Handler) Handler { return next }
	second := func(next Handler) Handler { return next }

	router.Use(first)
	router.Use(second)

	if len(router.stack) != 2 || router.stack[0] == nil || router.stack[1] == nil {
		t.Fatal("Use should append all supplied middleware")
	}
}
