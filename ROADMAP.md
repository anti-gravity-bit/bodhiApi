# bodhiApi — SDE II completion roadmap

The README already lists product ideas. This file is the **interview bar**, not a feature dump.

**Bar:** you can implement `sync.Once`-level lifecycle, middleware order, and 404 vs 405 from this source without Gin.

## Done

- [x] stdlib-only router, context, JSON errors
- [x] Recover, request ID, slog middleware
- [x] Graceful shutdown example
- [x] Public tests + CI
- [x] Gold-standard README
- [x] Route groups + group middleware
- [x] Double-write protection on `Context.JSON`
- [x] `Allow` header on 405
- [x] Request body limit
- [x] Timeout middleware via `context.WithTimeout`

## Remaining (pick in order; stop when interviews start)

### Must (if a week is free)

Must-list complete.

### Should

5. OpenAPI sketch from registered routes
6. Example with `/healthz` + `/readyz` split

### Won’t for SDE II

- Trie router rewrite unless tests stay green and README semantics are preserved
- WebSocket
- Template engine

## Interview checkpoints

- Linear scan vs trie: when n=20 routes, why you do not care
- Panic in handler vs panic after bytes written
- Why unknown `error` becomes a generic 500
- How `ListenAndServe` drains on Ctrl-C in `examples/todo`

Keep DSA time. Do not polish this repo for 30 days.
