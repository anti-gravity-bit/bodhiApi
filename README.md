# bodhiApi

`bodhiApi` is a small, readable Go framework for building HTTP APIs.

The name **bodhiApi** is inspired by my son’s name, **Sambodhi**, and by *bodhi*, a Buddhist term associated with awakening, understanding, and clarity. This project carries that inspiration as an engineering philosophy: make complexity visible, seek understanding before abstraction, and build software that is useful, reliable, and considerate of the people who depend on it.

The project is intentionally being developed in the open and in small, understandable steps. Its goal is not to hide HTTP behind a large abstraction layer. Its goal is to provide a dependable API foundation while keeping routing, request context, middleware, errors, and server lifecycle easy to inspect.

> Current release: **0th release / pre-alpha**
>
> The API is experimental. Names, behavior, and package layout may change before the first stable release.

## Why bodhiApi?

Many API frameworks optimize for feature count first. bodhiApi takes a different approach:

- **Readable core:** routing and request dispatch use straightforward data structures and control flow.
- **Small surface area:** handlers receive a focused `Context` instead of a large framework object.
- **Standard library alignment:** the application exposes `net/http` handlers and uses standard Go HTTP types.
- **Explicit behavior:** static routes, parameter routes, middleware order, error conversion, and server shutdown are visible in the code.
- **Production-minded direction:** the initial core is small, but the roadmap explicitly targets operational safety, observability, security, testing, and compatibility.
- **Continuous project:** the 0th release is a working foundation, not a claim that the framework is complete.

The project is unique mainly in its priorities: it treats comprehensibility as a feature. A contributor should be able to follow a request from the router to middleware to context to handler to response without learning a large internal runtime.

## The philosophy behind the name

The Buddhist idea of *bodhi* is used here as inspiration, not as a claim that software can reproduce spiritual practice. It gives the project a set of practical engineering reminders:

- **Clarity over unnecessary complexity:** understand the request path, data flow, and failure modes before adding abstractions.
- **Mindful simplicity:** keep the framework small enough that maintainers can reason about its behavior.
- **Right effort:** improve correctness and production safety continuously, without adding features merely for appearance.
- **Compassion through reliability:** treat predictable uptime, safe errors, clear documentation, and respectful data handling as ways of caring for users and operators.
- **Non-attachment to early design:** the 0th release is a beginning. Designs should evolve when evidence, tests, and real usage show a better path.
- **Interdependence:** APIs connect clients, services, databases, operators, and people. Changes should consider the whole system rather than only the local function.
- **Continuous learning:** every bug, test, review, and production observation should improve the framework.

These principles are intentionally translated into engineering practices: explicit code, small interfaces, stable error contracts, strong tests, observability, secure defaults, and honest documentation.

## Installation

The current module path is:

```text
github.com/anti-gravity-bit/bodhiApi
```

Because this is the 0th release, use a version or commit explicitly while the API evolves:

```bash
go get github.com/anti-gravity-bit/bodhiApi
```

The package name is `bodhiApi`.

## Quick start

```go
package main

import (
	"net/http"

	bodhiApi "github.com/anti-gravity-bit/bodhiApi"
)

func main() {
	app := bodhiApi.New(bodhiApi.Info{
		Title:   "Users API",
		Version: "0.0.0",
	})

	app.GET("/users/:id", func(c bodhiApi.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"id": c.Param("id"),
		})
	})

	app.POST("/users", func(c bodhiApi.Context) error {
		return c.JSON(http.StatusCreated, map[string]string{
			"status": "created",
		})
	})

	if err := app.ListenAndServe(":8080"); err != nil {
		panic(err)
	}
}
```

The built-in `GET /health` route returns:

```json
{"status":"ok"}
```

## Request handling model

A request follows this path:

1. `App.Handler` receives the standard `net/http` request.
2. The router compares the HTTP method and URL path against registered routes.
3. Exact static routes take precedence over parameter routes.
4. Middleware wraps the selected handler.
5. A request-scoped `Context` is created.
6. The handler reads parameters, query values, and headers or writes JSON/status responses.
7. Returned errors are converted to `HTTPError` responses when no response has already been written.

This model is deliberately simple enough to debug with ordinary Go tools.

## Routing

### Current routing behavior

Routes are registered with the convenience methods `GET`, `POST`, `PUT`, `PATCH`, and `DELETE`, or directly with `Handle`.

```go
app.GET("/users", listUsers)
app.GET("/users/:id", getUser)
app.POST("/users", createUser)
```

Current rules:

- Paths must begin with `/`.
- Routes are matched by HTTP method and path segments.
- A segment beginning with `:` is a parameter.
- Parameters are available through `c.Param("name")`.
- Static routes are preferred over parameter routes.
- The first matching parameter route is used as the fallback.
- A trailing slash remains significant (`/users` and `/users/` are different).
- A matching path with a different method produces a 405 error with allowed methods.
- An unknown path produces a 404 error.

### Production routing table target

The current implementation uses a slice and linear scan because it is easy to understand. For production-scale routing, the next routing-table design should preserve the same semantics while improving lookup, validation, and introspection.

| Capability | 0th release | Production target |
|---|---|---|
| Static routes | Supported | Trie/radix lookup with deterministic precedence |
| Named parameters | Supported with `:name` | Validated names, collision checks, typed constraints |
| Wildcards | Not yet supported | Explicit catch-all syntax with strict precedence |
| HTTP methods | GET, POST, PUT, PATCH, DELETE | Full standard method set plus custom methods |
| Method mismatch | 405 error details | 405 response with `Allow` header |
| Route precedence | Static before parameter | Static, constrained parameter, wildcard precedence table |
| Trailing slash | Significant | Configurable strict, redirect, or canonical mode |
| Duplicate routes | Not explicitly rejected | Registration-time conflict detection |
| Route groups | Not yet supported | Prefix, middleware, tags, and version groups |
| API versioning | Manual paths | First-class `/v1` groups and negotiated version policy |
| Route inspection | Internal route slice | Public route table/debug endpoint and generated documentation |
| Matching cost | Linear scan | Near O(path segments) lookup |
| Route constraints | None | UUID, integer, regex, and custom predicate constraints |
| Host matching | Not yet supported | Optional host/subdomain routing |
| Content negotiation | Not yet supported | Explicit media-type and format matching |

A production routing table should also define conflicts before the server starts. Ambiguous patterns should fail during registration rather than depend on registration order.

## Middleware

Middleware has the form:

```go
type Middleware func(Handler) Handler
```

Example:

```go
app.Use(func(next bodhiApi.Handler) bodhiApi.Handler {
	return func(c bodhiApi.Context) error {
		// before
		err := next(c)
		// after
		return err
	}
})
```

The current default middleware stack includes:

- `Recover`: converts panics to an internal server error.
- `RequestID`: reuses or generates `X-Request-ID`.
- `Logger`: emits structured `slog` request logs.

Recommended future middleware includes authentication, authorization, CORS, compression, rate limiting, request size limits, timeout enforcement, metrics, tracing, and audit logging.

## Errors

Use `HTTPError` when a handler needs to return a deliberate HTTP response:

```go
return bodhiApi.NewHTTPError(
	http.StatusBadRequest,
	"email is required",
)
```

Helpers are available for common cases:

```go
return bodhiApi.NotFound("user not found")
return bodhiApi.MethodNotAllowed("GET, POST")
```

`HTTPError` contains:

- `Status`: HTTP response status code.
- `Message`: human-readable error message.
- `Code`: stable machine-readable code.
- `Details`: optional structured metadata.

Unknown errors are intentionally converted to a generic internal error so implementation details are not exposed to clients.

## 0th-release scope

The 0th release currently provides:

- A small `net/http` compatible application type.
- Basic static and parameter routing.
- GET, POST, PUT, PATCH, and DELETE registration helpers.
- Request context access for path parameters, query values, and headers.
- JSON and status response helpers.
- Request-scoped context storage.
- Middleware composition.
- Panic recovery.
- Request IDs.
- Structured request logging with `log/slog`.
- HTTP error types and JSON error responses.
- Graceful server shutdown.
- Unit and integration-style tests around the current behavior.

It does not yet promise stable APIs, full production hardening, OpenAPI generation, database integration, authentication, or a complete observability stack.

## Next targets

The next development targets are:

1. **Routing table v1**
   - Replace linear route matching with a deterministic trie/radix structure.
   - Add duplicate and ambiguous route validation.
   - Add route groups and group-level middleware.
   - Add full method support and correct `Allow` headers.

2. **Request and response safety**
   - Add request body size limits.
   - Add configurable request timeouts.
   - Prevent accidental double writes.
   - Add typed binding and validation with explicit error messages.

3. **Production observability**
   - Add metrics hooks and Prometheus-compatible metrics.
   - Add OpenTelemetry tracing hooks.
   - Add correlation IDs and trace propagation.
   - Add structured access-log fields for status and response size.

4. **Security defaults**
   - Add secure headers.
   - Add CORS policy middleware.
   - Add rate limiting.
   - Add authentication and authorization extension points.
   - Add trusted-proxy and forwarded-header configuration.

5. **API contract tooling**
   - Add OpenAPI generation.
   - Add request/response schema validation.
   - Add versioning and deprecation metadata.
   - Add generated client and documentation workflows.

6. **Operational maturity**
   - Add readiness and liveness endpoints.
   - Add graceful drain behavior.
   - Add configurable server limits.
   - Add deployment examples for containers and orchestration platforms.

## Production-level standards checklist

To become a production-level API development framework, bodhiApi should implement or document the following standards.

### HTTP and routing

- Correct status codes and method semantics.
- `Allow` headers for 405 responses.
- RFC-compatible path and query handling.
- Configurable trailing-slash policy.
- Route conflict detection.
- Request URI and header size limits.
- Content negotiation.
- Proper `HEAD`, `OPTIONS`, and `CONNECT` handling where applicable.
- HTTP/2 and HTTP/3 deployment guidance.

### Security

- TLS configuration guidance and secure defaults.
- HSTS, CSP, `X-Content-Type-Options`, and related security headers.
- CORS configuration with explicit origin policy.
- CSRF guidance for cookie-authenticated APIs.
- Authentication and authorization interfaces.
- Input validation and output encoding.
- Request body and multipart upload limits.
- Rate limiting and abuse protection.
- Secret management guidance.
- Safe error redaction.
- Dependency and supply-chain scanning.

### Reliability

- Context cancellation propagation.
- Per-request deadlines and timeouts.
- Graceful shutdown and connection draining.
- Retry guidance with idempotency rules.
- Circuit breaker and bulkhead extension points.
- Health, readiness, and startup probes.
- Backpressure and bounded concurrency.
- Clear behavior for partial writes and client disconnects.

### Observability

- Structured logs with stable field names.
- Request ID and trace ID propagation.
- Metrics for request count, latency, status, errors, and in-flight requests.
- Distributed tracing integration.
- Status and response-size capture.
- Log redaction rules for credentials and personal data.
- Debug endpoints disabled or protected in production.

### API design

- OpenAPI 3.x generation and validation.
- Consistent error envelope.
- Pagination, filtering, and sorting conventions.
- Idempotency keys for unsafe retryable operations.
- API versioning and deprecation policy.
- Compatibility and migration policy.
- Standard date, time, and identifier formats.
- ETag and conditional request support where useful.

### Testing and delivery

- Unit, integration, and end-to-end tests.
- Race detector and fuzz testing.
- Contract and compatibility tests.
- Load and soak testing.
- Static analysis and vulnerability scanning.
- Reproducible builds.
- Semantic versioning after the first stable release.
- Changelog and migration guides.
- CI checks for formatting, tests, race detection, and security.

## Development

Run formatting and tests from the module root:

```bash
gofmt -w .
go test ./...
go test -race ./...
```

The project is intentionally small. Before adding a feature, prefer a clear extension point and a focused test over a large abstraction. Keep behavior explicit, document compatibility implications, and update this README when a roadmap target becomes implemented.

## Project status

`bodhiApi` is under continuous development. The 0th release should be treated as an experimental foundation for learning, prototyping, and contributing—not as a promise of production readiness. Production use should wait until the routing, security, observability, reliability, and compatibility targets above are implemented and documented.
