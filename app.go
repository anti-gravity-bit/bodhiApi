package bodhiApi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// Info identifies an application. It is also the starting point for future
// API documentation metadata.
type Info struct {
	Title   string
	Version string
}

// App owns the router, middleware stack, logger, and HTTP server lifecycle.
//
// Applications can be used in two ways:
//   - Handler returns a standard net/http handler for embedding or testing.
//   - ListenAndServe or Listen starts a server that can later be stopped with
//     Shutdown.
type App struct {
	Info   Info
	router *Router
	log    *slog.Logger
	server *http.Server
}

// New creates an application with the default middleware stack and a health
// endpoint.
//
// The default middleware order is Recover, RequestID, then Logger. Because the
// router applies middleware from the inside out, Recover is the outermost layer
// and can catch panics from all later layers.
func New(info Info) *App {
	router := NewRouter()
	router.Use(Recover, RequestID, Logger(slog.Default()))

	app := &App{
		Info:   info,
		router: router,
		log:    slog.Default(),
	}

	app.router.GET("/health", health)
	return app
}

// health is the built-in liveness endpoint.
func health(c Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Router returns the application's router for advanced route configuration.
func (a *App) Router() *Router {
	return a.router
}

// GET registers a GET route on the application's router.
func (a *App) GET(path string, h Handler) {
	a.router.GET(path, h)
}

// POST registers a POST route on the application's router.
func (a *App) POST(path string, h Handler) {
	a.router.POST(path, h)
}

// PUT registers a PUT route on the application's router.
func (a *App) PUT(path string, h Handler) {
	a.router.PUT(path, h)
}

// PATCH registers a PATCH route on the application's router.
func (a *App) PATCH(path string, h Handler) {
	a.router.PATCH(path, h)
}

// DELETE registers a DELETE route on the application's router.
func (a *App) DELETE(path string, h Handler) {
	a.router.DELETE(path, h)
}

// Use adds middleware to the application's router.
func (a *App) Use(mw ...Middleware) {
	a.router.Use(mw...)
}

// Handler adapts the application to the standard net/http Handler interface.
// This allows an App to be mounted on another mux or exercised with
// httptest without opening a network listener.
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(a.serve)
}

// serve performs the complete request lifecycle:
//  1. Match the method and path.
//  2. Select a route handler, 405 handler, or 404 handler.
//  3. Apply application middleware.
//  4. Create the request Context.
//  5. Execute the handler.
//  6. Convert an unhandled error into an HTTP response.
func (a *App) serve(w http.ResponseWriter, request *http.Request) {
	match := a.router.match(request.Method, request.URL.Path)
	handler := a.handlerForMatch(match)
	handler = a.router.apply(handler)

	context := newContext(w, request, match.params)
	if err := handler(context); err != nil && !context.wrote {
		writeError(w, err)
	}
}

// handlerForMatch turns the router result into a handler that can be executed
// by the common middleware and response pipeline.
func (a *App) handlerForMatch(match matchResult) Handler {
	if match.found {
		return match.handler
	}

	if len(match.allow) > 0 {
		allow := joinAllow(match.allow)
		return func(Context) error {
			return MethodNotAllowed(allow)
		}
	}

	return func(Context) error {
		return NotFound("not found")
	}
}

// joinAllow formats the methods associated with a matching path for the
// Allow response header and 405 error details.
func joinAllow(methods []string) string {
	return strings.Join(methods, ", ")
}

// ListenAndServe creates a configured HTTP server, binds addr, and blocks
// until the server stops. Call Shutdown from a signal handler or another
// goroutine to stop it gracefully.
func (a *App) ListenAndServe(addr string) error {
	a.server = a.newServer(addr)
	a.log.Info("listening", slog.String("addr", addr), slog.String("title", a.Info.Title))

	return normalizeServerError(a.server.ListenAndServe())
}

// Listen serves requests on an already-open listener. This is useful for tests
// and for applications that need to control listener creation themselves.
func (a *App) Listen(listener net.Listener) error {
	a.server = &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return normalizeServerError(a.server.Serve(listener))
}

// Shutdown gracefully stops the current server. If the application has not
// started a server, there is nothing to shut down and nil is returned.
func (a *App) Shutdown(ctx context.Context) error {
	if a.server == nil {
		return nil
	}

	return a.server.Shutdown(ctx)
}

// newServer creates the fully configured server used by ListenAndServe.
// Keeping server configuration in one helper makes the timeout policy easy to
// inspect and prevents Handler setup from being duplicated.
func (a *App) newServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// normalizeServerError treats the standard graceful-shutdown sentinel as a
// successful shutdown while preserving all real server errors.
func normalizeServerError(err error) error {
	if err == http.ErrServerClosed {
		return nil
	}

	return err
}

// writeError converts an application error into a JSON HTTP response. Unknown
// errors are intentionally hidden behind the generic internal-server-error
// response returned by AsHTTPError.
func writeError(w http.ResponseWriter, err error) {
	httpError := AsHTTPError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpError.Status)
	_ = json.NewEncoder(w).Encode(httpError)
}
