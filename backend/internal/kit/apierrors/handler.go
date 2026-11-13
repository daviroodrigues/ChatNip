package apierrors

import (
	"encoding/json"
	"errors"
	"github.com/rs/zerolog"
	"net/http"
)

func HandleError(w http.ResponseWriter, err error, logger zerolog.Logger) {
	var appErr *AppError

	if errors.As(err, &appErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(appErr.Code)
		json.NewEncoder(w).Encode(appErr)
		return
	}

	logger.Error().Err(err).Msg("Unhandled internal server error")

	internalErr := NewInternalServerError("internal server error")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(internalErr.Code)
	json.NewEncoder(w).Encode(internalErr)
}
