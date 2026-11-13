package apierrors

import (
	"github.com/rs/zerolog"
	"net/http"
)

type APIHandler func(w http.ResponseWriter, r *http.Request) error

func (h APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		logger := zerolog.Ctx(r.Context())
		HandleError(w, err, *logger)
	}
}
