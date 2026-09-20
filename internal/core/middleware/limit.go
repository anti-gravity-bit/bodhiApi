package middleware

import (
	"net/http"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
)

// MaxBytes limits the request body to maxBytes using http.MaxBytesReader.
// Downstream reads that exceed the limit fail; the original request pointer is
// kept and only Body is replaced.
func MaxBytes(maxBytes int64) core.Middleware {
	return func(nextHandler core.Handler) core.Handler {
		return func(requestContext core.Context) error {
			httpRequest := requestContext.Request()
			httpRequest.Body = http.MaxBytesReader(requestContext.ResponseWriter(), httpRequest.Body, maxBytes)
			return nextHandler(requestContext)
		}
	}
}
