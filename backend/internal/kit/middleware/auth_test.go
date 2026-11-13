package middleware_test

import (
	"chatnip/internal/kit/auth"
	"chatnip/internal/kit/database"
	"chatnip/internal/kit/middleware"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	dbPath := "/tmp/test_middleware.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	db, err := database.NewSQLiteConnection(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite connection: %v", err)
	}
	defer db.Close()

	store := auth.NewSQLStore(db)
	internalKey := "master-secret-123"

	// Create a valid API key
	rawKey, _, err := store.Create(context.Background(), "Test Client")
	if err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	t.Run("InternalAuth - Success with X-Internal-Key", func(t *testing.T) {
		handler := middleware.InternalAuth(internalKey)(successHandler)
		req := httptest.NewRequest(http.MethodPost, "/auth/keys", nil)
		req.Header.Set("X-Internal-Key", internalKey)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("InternalAuth - Unauthorized without X-Internal-Key", func(t *testing.T) {
		handler := middleware.InternalAuth(internalKey)(successHandler)
		req := httptest.NewRequest(http.MethodPost, "/auth/keys", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("ApiKeyAuth - Success with X-API-Key", func(t *testing.T) {
		handler := middleware.ApiKeyAuth(store, internalKey)(successHandler)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/send", nil)
		req.Header.Set("X-API-Key", rawKey)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("ApiKeyAuth - Success with Bearer token", func(t *testing.T) {
		handler := middleware.ApiKeyAuth(store, internalKey)(successHandler)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/send", nil)
		req.Header.Set("Authorization", "Bearer "+rawKey)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("ApiKeyAuth - Success with Master Internal Key override", func(t *testing.T) {
		handler := middleware.ApiKeyAuth(store, internalKey)(successHandler)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/send", nil)
		req.Header.Set("X-Internal-Key", internalKey)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("ApiKeyAuth - Unauthorized with Invalid API Key", func(t *testing.T) {
		handler := middleware.ApiKeyAuth(store, internalKey)(successHandler)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/send", nil)
		req.Header.Set("X-API-Key", "invalid_key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})
}
