package auth

import (
	"chatnip/internal/kit/apierrors"
	httpPlatform "chatnip/internal/kit/http"
	"chatnip/internal/kit/middleware"
	"chatnip/internal/kit/router"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"net/http"
	"time"
)

type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	MaskedKey string     `json:"masked_key"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type CreateKeyResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key"`
	MaskedKey string    `json:"masked_key"`
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	Create(ctx context.Context, name string) (string, *APIKey, error)
	Validate(ctx context.Context, rawKey string) (bool, error)
	List(ctx context.Context) ([]*APIKey, error)
	Revoke(ctx context.Context, id string) error
}

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) *SQLStore {
	return &SQLStore{db: db}
}

func MaskAPIKey(rawKey string) string {
	if len(rawKey) < 16 {
		return "****"
	}
	prefix := rawKey[:12]
	suffix := rawKey[len(rawKey)-4:]
	return fmt.Sprintf("%s...%s", prefix, suffix)
}

func (s *SQLStore) Create(ctx context.Context, name string) (string, *APIKey, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, err
	}
	rawKey := "cn_live_" + hex.EncodeToString(bytes)
	keyPrefix := rawKey[:16]
	maskedKey := MaskAPIKey(rawKey)

	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return "", nil, err
	}
	id := fmt.Sprintf("key_%s", hex.EncodeToString(idBytes))

	argonHash, err := HashWithArgon(rawKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to hash api key with argon2: %w", err)
	}

	query := `INSERT INTO api_keys (id, name, key_hash, key_prefix, masked_key) VALUES (?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, query, id, name, argonHash, keyPrefix, maskedKey); err != nil {
		return "", nil, err
	}

	apiKey := &APIKey{
		ID:        id,
		Name:      name,
		MaskedKey: maskedKey,
		CreatedAt: time.Now().UTC(),
	}

	return rawKey, apiKey, nil
}

func (s *SQLStore) Validate(ctx context.Context, rawKey string) (bool, error) {
	if len(rawKey) < 16 {
		return false, nil
	}

	prefix := rawKey[:16]

	query := `SELECT id, key_hash FROM api_keys WHERE key_prefix = ? AND revoked_at IS NULL`
	rows, err := s.db.QueryContext(ctx, query, prefix)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, storedHash string
		if err := rows.Scan(&id, &storedHash); err != nil {
			return false, err
		}

		valid, err := VerifyWithArgon(rawKey, storedHash)
		if err == nil && valid {
			return true, nil
		}
	}

	return false, nil
}

func (s *SQLStore) List(ctx context.Context) ([]*APIKey, error) {
	query := `SELECT id, name, COALESCE(NULLIF(masked_key, ''), key_prefix || '...'), created_at, revoked_at FROM api_keys ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*APIKey
	for rows.Next() {
		var (
			k         APIKey
			revokedAt sql.NullTime
		)
		if err := rows.Scan(&k.ID, &k.Name, &k.MaskedKey, &k.CreatedAt, &revokedAt); err != nil {
			return nil, err
		}
		if revokedAt.Valid {
			k.RevokedAt = &revokedAt.Time
		}
		list = append(list, &k)
	}

	return list, nil
}

func (s *SQLStore) Revoke(ctx context.Context, id string) error {
	query := `UPDATE api_keys SET revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND revoked_at IS NULL`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return apierrors.NewNotFound("api key")
	}
	return nil
}

type Handler struct {
	store  Store
	logger zerolog.Logger
}

func NewHandler(store Store, logger zerolog.Logger) *Handler {
	return &Handler{
		store:  store,
		logger: logger,
	}
}

type CreateKeyRequest struct {
	Name string `json:"name"`
}

// @Summary      Criar nova API Key
// @Description  Gera uma chave de API para um cliente com hash Argon2id (exibida em texto claro apenas nesta resposta)
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     InternalKeyAuth
// @Param        request body CreateKeyRequest true "Dados da nova chave"
// @Success      201 {object} CreateKeyResponse
// @Failure      400 {object} apierrors.AppError
// @Failure      401 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Router       /api/v1/auth/keys [post]
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) error {
	var req CreateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		return apierrors.NewBadRequest("field 'name' is required")
	}

	rawKey, key, err := h.store.Create(r.Context(), req.Name)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to generate API Key with Argon2")
		return apierrors.NewInternalServerError("could not generate api key")
	}

	resp := CreateKeyResponse{
		ID:        key.ID,
		Name:      key.Name,
		APIKey:    rawKey,
		MaskedKey: key.MaskedKey,
		CreatedAt: key.CreatedAt,
	}

	httpPlatform.JSON(w, http.StatusCreated, resp)
	return nil
}

// @Summary      Listar API Keys
// @Description  Retorna as chaves de API cadastradas com chave mascarada
// @Tags         Auth
// @Produce      json
// @Security     InternalKeyAuth
// @Success      200 {array} APIKey
// @Failure      401 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Router       /api/v1/auth/keys [get]
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) error {
	keys, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to list API Keys")
		return apierrors.NewInternalServerError("could not list api keys")
	}

	if keys == nil {
		keys = []*APIKey{}
	}

	httpPlatform.JSON(w, http.StatusOK, keys)
	return nil
}

// @Summary      Revogar API Key
// @Description  Revoga uma chave de API existente pelo ID
// @Tags         Auth
// @Produce      json
// @Security     InternalKeyAuth
// @Param        id path string true "ID da chave"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      401 {object} apierrors.AppError
// @Failure      404 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Router       /api/v1/auth/keys/{id} [delete]
func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if id == "" {
		return apierrors.NewBadRequest("key id is required")
	}

	if err := h.store.Revoke(r.Context(), id); err != nil {
		return err
	}

	httpPlatform.JSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "API key revoked successfully",
	})
	return nil
}

func (h *Handler) RegisterRoutes(r chi.Router, apiVersion string, internalKey string) {
	r.Route(router.ApiPath(apiVersion, "/auth/keys"), func(r chi.Router) {
		r.Use(middleware.InternalAuth(internalKey))

		r.Post("/", apierrors.APIHandler(h.handleCreate).ServeHTTP)
		r.Get("/", apierrors.APIHandler(h.handleList).ServeHTTP)
		r.Delete("/{id}", apierrors.APIHandler(h.handleRevoke).ServeHTTP)
	})
}
