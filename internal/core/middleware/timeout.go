package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
)

// requestSetter is implemented by RequestContext so Timeout can replace the
// request with a context-derived copy without widening core.Context.
type requestSetter interface {
	SetRequest(*http.Request)
}

// Timeout sets a deadline on the request context for downstream handlers.
// Handlers that observe Request().Context() see cancellation when duration
// elapses. The middleware waits for the wrapped handler to return.
func Timeout(duration time.Duration) core.Middleware {
	return func(nextHandler core.Handler) core.Handler {
		return func(requestContext core.Context) error {
			httpRequest := requestContext.Request()
			timeoutContext, cancel := context.WithTimeout(httpRequest.Context(), duration)
			defer cancel()
			if setter, implementsSetter := requestContext.(requestSetter); implementsSetter {
				setter.SetRequest(httpRequest.WithContext(timeoutContext))
			}
			return nextHandler(requestContext)
		}
	}
}
