# Contributing to bodhiApi

This guide is for human contributors and coding agents.

## Design rules

1. Keep the module import path stable: `github.com/anti-gravity-bit/bodhiApi`.
2. Treat the root package as the public compatibility facade.
3. Put implementation details under `internal/core`.
4. Keep dependencies flowing inward:

```text
public facade -> app -> router/context/middleware/errors
                         \
                          -> standard library
```

5. Do not import the root package from `internal/core`; that would create an
   import cycle.
6. Preserve existing HTTP behavior unless a change is explicitly documented.
7. Add tests for behavior before changing implementation.
8. Prefer small, named helpers over clever abstractions.
9. Comments should explain contracts, invariants, or non-obvious trade-offs, and
   document every exported type and function.
10. Use `gofmt`. Keep function signatures on one line. Single-statement functions
    should be written as one-liners.
11. Names must be meaningful. Do not use single-character identifiers. Prefer
    `requestContext`, `handlerError`, `routerInstance`, and `test *testing.T`
    over `c`, `err` when a more specific name exists, `r`, and `t`.

## Directory guide

- `LICENSE`: MIT license (required for OSI / r/opensource).
- `api.go`: exported aliases, constructors, and compatibility functions.
- `example_test.go`: godoc examples for pkg.go.dev.
- `examples/todo`: runnable API with graceful shutdown (`go run ./examples/todo`).
- `.github/workflows/test.yml`: `go test ./...` on push.
- `.github/social-preview.png`: 1280×640 image for Settings → Social preview.
- `internal/core/app`: application lifecycle and request dispatch.
- `internal/core/context`: request-scoped state and response helpers.
- `internal/core/errors`: public error shape and safe conversion.
- `internal/core/middleware`: built-in middleware and request metadata.
- `internal/core/router`: route registration, matching, and middleware order.
- `api_test.go`: public integration tests.

## GitHub repository settings

These are not encoded in git; set them on github.com after push:

1. **Topics** (About → gear): `go`, `golang`, `http`, `router`, `middleware`, `net-http`, `json-api`.
2. **Social preview** (Settings → General → Social preview): upload `.github/social-preview.png`
   (1280×640 PNG, under 1 MB).

## Change workflow

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
```

When changing routing, test static routes, parameter routes, trailing slashes,
method mismatches, and registration order. When changing response behavior, test
status, headers, body, and returned errors.

## Guidance for agents

Before editing:

1. Read `README.md` and this file.
2. Identify the owning package from the directory guide.
3. Search for public aliases in `api.go` and callers in tests.
4. Make the smallest change that satisfies the requirement.

After editing:

1. Run `gofmt`.
2. Run focused tests, then `go test ./...`.
3. Run diagnostics or `go vet ./...`.
4. Summarize changed files and validation results.
5. Do not commit or push unless explicitly requested.
