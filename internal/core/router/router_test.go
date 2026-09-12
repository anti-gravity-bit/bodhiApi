package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
)

func TestStaticRoutesBeatParameterRoutes(test *testing.T) {
	routerInstance := New()
	parameterHandler := func(core.Context) error { return nil }
	staticHandler := func(core.Context) error { return nil }
	routerInstance.GET("/users/:id", parameterHandler)
	routerInstance.GET("/users/me", staticHandler)

	matchResult := routerInstance.Match(http.MethodGet, "/users/me")
	if !matchResult.Found || reflect.ValueOf(matchResult.Handler).Pointer() != reflect.ValueOf(staticHandler).Pointer() {
		test.Fatal("static route did not win")
	}
}

func TestParameterRoutesPreserveFirstRegistration(test *testing.T) {
	routerInstance := New()
	firstHandler := func(core.Context) error { return nil }
	secondHandler := func(core.Context) error { return nil }
	routerInstance.GET("/items/:first", firstHandler)
	routerInstance.GET("/items/:second", secondHandler)

	matchResult := routerInstance.Match(http.MethodGet, "/items/value")
	if reflect.ValueOf(matchResult.Handler).Pointer() != reflect.ValueOf(firstHandler).Pointer() {
		test.Fatal("first parameter route did not win")
	}
	if !reflect.DeepEqual(matchResult.Params, map[string]string{"first": "value"}) {
		test.Fatalf("params = %#v", matchResult.Params)
	}
}

func TestRouterReturnsAllowedMethods(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/users", func(core.Context) error { return nil })
	routerInstance.POST("/users", func(core.Context) error { return nil })
	routerInstance.POST("/users", func(core.Context) error { return nil })

	matchResult := routerInstance.Match(http.MethodDelete, "/users")
	if matchResult.Found || !reflect.DeepEqual(matchResult.Allow, []string{http.MethodGet, http.MethodPost}) {
		test.Fatalf("match = %#v", matchResult)
	}
}

func TestRouterDistinguishesPaths(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/users", func(core.Context) error { return nil })

	if !routerInstance.Match(http.MethodGet, "/users").Found {
		test.Fatal("expected exact path to match")
	}
	if routerInstance.Match(http.MethodGet, "/users/").Found {
		test.Fatal("trailing slash should remain significant")
	}
	if routerInstance.Match(http.MethodGet, "/missing").Found {
		test.Fatal("unknown path should not match")
	}
}

func TestRouterMethodHelpers(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/get", func(core.Context) error { return nil })
	routerInstance.POST("/post", func(core.Context) error { return nil })
	routerInstance.PUT("/put", func(core.Context) error { return nil })
	routerInstance.PATCH("/patch", func(core.Context) error { return nil })
	routerInstance.DELETE("/delete", func(core.Context) error { return nil })

	for method, path := range map[string]string{http.MethodGet: "/get", http.MethodPost: "/post", http.MethodPut: "/put", http.MethodPatch: "/patch", http.MethodDelete: "/delete"} {
		if !routerInstance.Match(method, path).Found {
			test.Fatalf("%s %s did not match", method, path)
		}
	}
}

func TestRouterMatchesRootAndEmptyPath(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/", func(core.Context) error { return nil })
	if !routerInstance.Match(http.MethodGet, "/").Found {
		test.Fatal("root path should match")
	}
	if !routerInstance.Match(http.MethodGet, "").Found {
		test.Fatal("empty path should match the registered root route")
	}
}

func TestRouterExtractsMultipleParameters(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/users/:userId/posts/:postId", func(core.Context) error { return nil })
	matchResult := routerInstance.Match(http.MethodGet, "/users/ada/posts/99")
	if !matchResult.Found {
		test.Fatal("expected parameterized path to match")
	}
	expectedParameters := map[string]string{"userId": "ada", "postId": "99"}
	if !reflect.DeepEqual(matchResult.Params, expectedParameters) {
		test.Fatalf("params = %#v", matchResult.Params)
	}
}

func TestRouterDoesNotMatchDifferentSegmentCounts(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/users/:id", func(core.Context) error { return nil })
	if routerInstance.Match(http.MethodGet, "/users").Found {
		test.Fatal("shorter path should not match")
	}
	if routerInstance.Match(http.MethodGet, "/users/1/extra").Found {
		test.Fatal("longer path should not match")
	}
}

func TestRegisteredTrailingSlashPatternMatchesTrimmedPath(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/users/", func(core.Context) error { return nil })
	if !routerInstance.Match(http.MethodGet, "/users").Found {
		test.Fatal("registered /users/ currently matches /users after pattern trim")
	}
	if routerInstance.Match(http.MethodGet, "/users/").Found {
		test.Fatal("request /users/ should not match because trailing slash is significant on requests")
	}
}

func TestRouterMiddlewareRunsInRegistrationOrder(test *testing.T) {
	var callOrder []string
	middlewareFactory := func(name string) core.Middleware {
		return func(nextHandler core.Handler) core.Handler {
			return func(requestContext core.Context) error {
				callOrder = append(callOrder, name+" before")
				handlerError := nextHandler(requestContext)
				callOrder = append(callOrder, name+" after")
				return handlerError
			}
		}
	}
	routerInstance := New()
	routerInstance.Use(middlewareFactory("first"), middlewareFactory("second"))
	wrappedHandler := routerInstance.Apply(func(core.Context) error {
		callOrder = append(callOrder, "handler")
		return nil
	})
	if err := wrappedHandler(nil); err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(callOrder, []string{"first before", "second before", "handler", "second after", "first after"}) {
		test.Fatalf("call order = %#v", callOrder)
	}
}

func TestRouterApplyWithNoMiddlewareReturnsHandler(test *testing.T) {
	routerInstance := New()
	originalHandler := func(core.Context) error { return errors.New("ok") }
	wrappedHandler := routerInstance.Apply(originalHandler)
	if err := wrappedHandler(nil); err == nil || err.Error() != "ok" {
		test.Fatalf("wrapped error = %v", err)
	}
}

func TestRouterHandlerUsesStandardHTTP(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/health", func(requestContext core.Context) error { return requestContext.Status(http.StatusNoContent) })
	responseRecorder := httptest.NewRecorder()
	routerInstance.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if responseRecorder.Code != http.StatusNoContent {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestRouterHandlerNotFoundAndMethodNotAllowed(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/users", func(requestContext core.Context) error { return requestContext.Status(http.StatusNoContent) })

	notFoundRecorder := httptest.NewRecorder()
	routerInstance.Handler().ServeHTTP(notFoundRecorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if notFoundRecorder.Code != http.StatusNotFound {
		test.Fatalf("not found status = %d", notFoundRecorder.Code)
	}
	if !strings.Contains(notFoundRecorder.Body.String(), `"code":"not_found"`) {
		test.Fatalf("not found body = %q", notFoundRecorder.Body.String())
	}

	methodRecorder := httptest.NewRecorder()
	routerInstance.Handler().ServeHTTP(methodRecorder, httptest.NewRequest(http.MethodPost, "/users", nil))
	if methodRecorder.Code != http.StatusMethodNotAllowed {
		test.Fatalf("method status = %d", methodRecorder.Code)
	}
	var methodBody map[string]any
	if err := json.Unmarshal(methodRecorder.Body.Bytes(), &methodBody); err != nil {
		test.Fatal(err)
	}
	if methodBody["code"] != "method_not_allowed" {
		test.Fatalf("method body = %#v", methodBody)
	}
}

func TestRouterHandlerDoesNotOverwriteWrittenResponse(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/written", func(requestContext core.Context) error {
		_ = requestContext.Status(http.StatusAccepted)
		return errors.New("late error")
	})
	responseRecorder := httptest.NewRecorder()
	routerInstance.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/written", nil))
	if responseRecorder.Code != http.StatusAccepted {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestRouterHandlerWritesUnhandledError(test *testing.T) {
	routerInstance := New()
	routerInstance.GET("/fail", func(core.Context) error { return errors.New("secret") })
	responseRecorder := httptest.NewRecorder()
	routerInstance.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/fail", nil))
	if responseRecorder.Code != http.StatusInternalServerError {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
	if strings.Contains(responseRecorder.Body.String(), "secret") {
		test.Fatalf("leaked error: %q", responseRecorder.Body.String())
	}
}

func TestRouterHandlerRunsMiddlewareForNotFound(test *testing.T) {
	routerInstance := New()
	middlewareRan := false
	routerInstance.Use(func(nextHandler core.Handler) core.Handler {
		return func(requestContext core.Context) error {
			middlewareRan = true
			return nextHandler(requestContext)
		}
	})
	responseRecorder := httptest.NewRecorder()
	routerInstance.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if !middlewareRan {
		test.Fatal("middleware should run for unmatched routes")
	}
	if responseRecorder.Code != http.StatusNotFound {
		test.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestRouteValidation(test *testing.T) {
	validationCases := []struct {
		name          string
		method        string
		path          string
		expectedPanic string
	}{
		{name: "missing leading slash", method: http.MethodGet, path: "users", expectedPanic: "bodhiApi: path must start with /"},
		{name: "empty path", method: http.MethodGet, path: "", expectedPanic: "bodhiApi: path must start with /"},
		{name: "missing method", method: "", path: "/users", expectedPanic: "bodhiApi: method required"},
	}
	for _, validationCase := range validationCases {
		test.Run(validationCase.name, func(subtest *testing.T) {
			defer func() {
				recoveredValue := recover()
				if recoveredValue != validationCase.expectedPanic {
					subtest.Fatalf("panic = %v", recoveredValue)
				}
			}()
			New().Handle(validationCase.method, validationCase.path, func(core.Context) error { return nil })
		})
	}
}

func TestParsePathPatternAndSplitRequestPath(test *testing.T) {
	if parsePathPattern("/") != nil {
		test.Fatalf("root pattern should parse to nil segments: %#v", parsePathPattern("/"))
	}
	parsed := parsePathPattern("/users/:id")
	if len(parsed) != 2 || parsed[0].raw != "users" || !parsed[1].isParameter || parsed[1].name != "id" {
		test.Fatalf("parsed = %#v", parsed)
	}

	if splitRequestPath("/") != nil || splitRequestPath("") != nil {
		test.Fatal("root and empty request paths should split to nil")
	}
	if !reflect.DeepEqual(splitRequestPath("/users/"), []string{"users", ""}) {
		test.Fatalf("trailing slash split = %#v", splitRequestPath("/users/"))
	}
	if !reflect.DeepEqual(splitRequestPath("/users"), []string{"users"}) {
		test.Fatalf("exact split = %#v", splitRequestPath("/users"))
	}
}

func TestMatchSegmentsLengthMismatch(test *testing.T) {
	matched, routeParameters := matchSegments([]pathSegment{{raw: "users"}}, []string{"users", "1"})
	if matched || routeParameters != nil {
		test.Fatalf("mismatch = %v %#v", matched, routeParameters)
	}
}

func TestAppendUniqueMethod(test *testing.T) {
	methods := appendUniqueMethod(nil, http.MethodGet)
	methods = appendUniqueMethod(methods, http.MethodGet)
	methods = appendUniqueMethod(methods, http.MethodPost)
	if !reflect.DeepEqual(methods, []string{http.MethodGet, http.MethodPost}) {
		test.Fatalf("methods = %#v", methods)
	}
}

func TestHandleRegistersCustomMethod(test *testing.T) {
	routerInstance := New()
	routerInstance.Handle("OPTIONS", "/users", func(core.Context) error { return nil })
	if !routerInstance.Match("OPTIONS", "/users").Found {
		test.Fatal("custom method should match")
	}
}
