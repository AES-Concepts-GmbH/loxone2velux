package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// NewLoggingMiddleware creates a request logging middleware. Successful GET
// requests (health checks, UI polling) are logged at debug level to keep the
// info log readable; everything else is logged at info level.
func NewLoggingMiddleware(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				status := ww.Status()
				event := logger.Info()
				if r.Method == http.MethodGet && status < 400 {
					event = logger.Debug()
				}
				event.
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("remote", r.RemoteAddr).
					Int("status", status).
					Int("bytes", ww.BytesWritten()).
					Dur("duration", time.Since(start)).
					Str("request_id", middleware.GetReqID(r.Context())).
					Msg("Request")
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// maxRequestBodyBytes caps request bodies. All JSON payloads of this API are
// a few hundred bytes; the limit only guards against oversized uploads.
const maxRequestBodyBytes = 1 << 20

// NewBodyLimitMiddleware rejects request bodies larger than limit bytes. The
// limit is enforced while the body is read, so handlers see a read error and
// answer 400 instead of buffering an unbounded payload.
func NewBodyLimitMiddleware(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NewTokenAuthMiddleware creates a middleware that validates API tokens.
// The token is read through tokenFn on every request so that a token changed
// at runtime takes effect immediately. An empty token disables authentication.
//
// Token can be provided via:
// - Header: Authorization: Bearer <token>
// - Query parameter: ?token=<token>  (for Loxone virtual outputs)
func NewTokenAuthMiddleware(tokenFn func() string, logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := tokenFn()
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			providedToken := ""

			// Check Authorization header first
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				providedToken = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// Fall back to query parameter (easier for Loxone)
			if providedToken == "" {
				providedToken = r.URL.Query().Get("token")
			}

			// Validate token using constant-time comparison
			if subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) != 1 {
				logger.Warn().
					Str("remote", r.RemoteAddr).
					Str("path", r.URL.Path).
					Msg("Unauthorized request - invalid token")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
