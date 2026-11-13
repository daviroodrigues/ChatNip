package health

import (
	httpPlatform "chatnip/internal/kit/http"
	"net/http"
)

type Pinger interface {
	Ping() error
}

type Handler struct {
	db Pinger
}

func NewHandler(db Pinger) *Handler {
	return &Handler{
		db: db,
	}
}

// @Summary      Verifica a saúde do serviço
// @Description  Retorna status 200 se a aplicação e o banco de dados estiverem respondendo
// @Tags         Health
// @Produce      json
// @Success      200 {object} map[string]string
// @Failure      503 {object} map[string]string
// @Router       /health [get]
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	if err := h.db.Ping(); err != nil {
		httpPlatform.JSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "error",
			"db":     "disconnected",
		})
		return
	}

	httpPlatform.JSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"db":     "connected",
	})
}
