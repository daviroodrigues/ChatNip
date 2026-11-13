package middleware

import (
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"net/http"
	"time"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriterWrapper(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{w, http.StatusOK}
}

func (rww *responseWriterWrapper) WriteHeader(code int) {
	rww.statusCode = code
	rww.ResponseWriter.WriteHeader(code)
}

func Logger(baseLogger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			reqID := uuid.NewString()
			path := r.URL.Path

			reqLogger := baseLogger.With().
				Str("request_id", reqID).
				Str("http_method", r.Method).
				Str("http_path", path).
				Str("remote_ip", r.RemoteAddr).
				Str("user_agent", r.UserAgent()).
				Logger()

			ctx := reqLogger.WithContext(r.Context())

			wrapper := newResponseWriterWrapper(w)

			next.ServeHTTP(wrapper, r.WithContext(ctx))

			latency := time.Since(startTime)

			routePattern := chi.RouteContext(r.Context()).RoutePattern()
			if routePattern == "" {
				if wrapper.statusCode == http.StatusNotFound {
					routePattern = "/{unknown_route_404}"
				} else {
					routePattern = path
				}
			}

			reqLogger.Info().
				Int("http_status", wrapper.statusCode).
				Dur("latency", latency).
				Str("route_pattern", routePattern).
				Msg("Request completed")
		})
	}
}
