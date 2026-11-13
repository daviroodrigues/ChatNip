package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type APIKeyValidator interface {
	Validate(ctx context.Context, rawKey string) (bool, error)
}

// InternalAuth restricts access exclusively to administrative callers providing X-Internal-Key
func InternalAuth(internalKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			providedKey := r.Header.Get("X-Internal-Key")
			if internalKey == "" || providedKey != internalKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Unauthorized: valid master key required in X-Internal-Key header",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ApiKeyAuth authenticates regular API calls using an API Key (via X-API-Key or Authorization Bearer),
// while allowing X-Internal-Key as a master administrative override.
func ApiKeyAuth(validator APIKeyValidator, internalKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Check master Internal Key override if provided
			internalHeader := r.Header.Get("X-Internal-Key")
			if internalKey != "" && internalHeader == internalKey {
				next.ServeHTTP(w, r)
				return
			}

			// 2. Extract API Key from headers (X-API-Key or Bearer)
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					apiKey = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if apiKey == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Unauthorized: API key required in X-API-Key or Authorization header",
				})
				return
			}

			// 3. Validate against storage
			valid, err := validator.Validate(r.Context(), apiKey)
			if err != nil || !valid {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Unauthorized: invalid or revoked API key",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Auth is maintained for backward compatibility, delegating to InternalAuth
func Auth(internalKey string) func(http.Handler) http.Handler {
	return InternalAuth(internalKey)
}
