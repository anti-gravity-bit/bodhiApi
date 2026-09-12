// Package middleware contains the default middleware shipped with bodhiApi.
//
// The default application stack is Recover, then RequestID, then Logger.
// Each function is independently reusable through the public facade.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/anti-gravity-bit/bodhiApi/internal/core"
	coreerrors "github.com/anti-gravity-bit/bodhiApi/internal/core/errors"
)

// HeaderRequestID is the HTTP header used to correlate a request across services.
const HeaderRequestID = "X-Request-ID"

// requestIDKey is the request-context storage key for the current request ID.
const requestIDKey = "request_id"

// Recover converts panics from downstream handlers into safe HTTP errors.
//
// If the wrapped handler panics, the panic is swallowed and the returned error
// is a 500 HTTPError with message "internal server error". Non-panic returns,
// including ordinary handler errors, pass through unchanged.
func Recover(nextHandler core.Handler) core.Handler {
	return func(requestContext core.Context) (handlerError error) {
		defer func() {
			if recover() != nil {
				handlerError = coreerrors.New(http.StatusInternalServerError, "internal server error")
			}
		}()
		return nextHandler(requestContext)
	}
}

// RequestID reuses an incoming request ID or generates a new one.
//
// The identifier is stored on the request context under "request_id" and written
// to the X-Request-ID response header so clients and log aggregators can correlate
// the exchange. An incoming header, when present, is trusted as-is.
func RequestID(nextHandler core.Handler) core.Handler {
	return func(requestContext core.Context) error {
		requestID := requestContext.Header(HeaderRequestID)
		if requestID == "" {
			requestID = newRequestID()
		}
		requestContext.Set(requestIDKey, requestID)
		requestContext.ResponseWriter().Header().Set(HeaderRequestID, requestID)
		return nextHandler(requestContext)
	}
}

// Logger emits one structured log record after the downstream handler returns.
//
// A nil logger is replaced with slog.Default(). Successes are logged at Info
// and failures at Error. Attributes include method, path, duration, optional
// request_id, and the error string when the handler failed.
func Logger(logger *slog.Logger) core.Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(nextHandler core.Handler) core.Handler {
		return func(requestContext core.Context) error {
			startTime := time.Now()
			handlerError := nextHandler(requestContext)
			logAttributes := []any{slog.String("method", requestContext.Request().Method), slog.String("path", requestContext.Request().URL.Path), slog.Duration("dur", time.Since(startTime))}
			if storedRequestID, found := requestContext.Get(requestIDKey); found {
				if requestID, isString := storedRequestID.(string); isString && requestID != "" {
					logAttributes = append(logAttributes, slog.String("request_id", requestID))
				}
			}
			if handlerError != nil {
				logger.Error("request", append(logAttributes, slog.String("err", handlerError.Error()))...)
			} else {
				logger.Info("request", logAttributes...)
			}
			return handlerError
		}
	}
}

// newRequestID returns an 8-byte random identifier encoded as 16 hex characters.
// If the system random reader fails, a stable fallback is used so request
// handling can continue without a cryptographic identifier.
func newRequestID() string {
	var randomBytes [8]byte
	if _, readError := rand.Read(randomBytes[:]); readError != nil {
		return "bodhiApi-unknown"
	}
	return hex.EncodeToString(randomBytes[:])
}
