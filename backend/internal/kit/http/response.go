package http

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Error().Err(err).Msg("Failed to encode JSON response")
		}
	}
}
