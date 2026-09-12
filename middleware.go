package bodhiApi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

// HeaderRequestID is the HTTP header used to propagate a request ID.
const HeaderRequestID = "X-Request-ID"

// ctxKeyRequestID is the private Context key used to share the request ID
// between RequestID, Logger, and application handlers.
const ctxKeyRequestID = "request_id"

// Recover converts panics from downstream middleware or handlers into an
// internal-server-error HTTPError. The panic is not rethrown, allowing the
// server process to continue serving other requests.
//
// Recover should normally be registered as the outermost middleware so it can
// observe panics from every other layer.
func Recover(next Handler) Handler {
	return func(c Context) (err error) {
		defer func() {
			if recover() != nil {
				err = NewHTTPError(http.StatusInternalServerError, "internal server error")
			}
		}()

		return next(c)
	}
}

// RequestID ensures every request has an ID.
//
// If the client sends X-Request-ID, that value is reused. Otherwise a new
// random ID is generated. The final ID is stored in Context and echoed in the
// response header so handlers, logs, and clients can refer to the same request.
func RequestID(next Handler) Handler {
	return func(c Context) error {
		requestID := c.Header(HeaderRequestID)
		if requestID == "" {
			requestID = newRequestID()
		}

		c.Set(ctxKeyRequestID, requestID)
		c.ResponseWriter().Header().Set(HeaderRequestID, requestID)

		return next(c)
	}
}

// Logger creates middleware that emits one slog record after the downstream
// handler returns.
//
// The record includes the request method, URL path, elapsed duration, and the
// request ID when RequestID middleware has populated it. Handler errors are
// logged at error level; successful requests are logged at info level.
func Logger(log *slog.Logger) Middleware {
	if log == nil {
		log = slog.Default()
	}

	return func(next Handler) Handler {
		return func(c Context) error {
			start := time.Now()
			err := next(c)

			attributes := requestLogAttributes(c, start)
			if err != nil {
				log.Error("request", append(attributes, slog.String("err", err.Error()))...)
			} else {
				log.Info("request", attributes...)
			}

			return err
		}
	}
}

// requestLogAttributes builds the fields shared by both successful and failed
// request log records.
func requestLogAttributes(c Context, start time.Time) []any {
	attributes := []any{
		slog.String("method", c.Request().Method),
		slog.String("path", c.Request().URL.Path),
		slog.Duration("dur", time.Since(start)),
	}

	requestID, ok := c.Get(ctxKeyRequestID)
	if requestID, isString := requestID.(string); ok && isString && requestID != "" {
		attributes = append(attributes, slog.String("request_id", requestID))
	}

	return attributes
}

// newRequestID creates a compact random hexadecimal request ID. If the system
// random source is unavailable, it returns a visible fallback rather than
// preventing the request from being processed.
func newRequestID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "bodhiApi-unknown"
	}

	return hex.EncodeToString(bytes[:])
}
