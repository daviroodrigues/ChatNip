package server

import (
	"context"
	"github.com/rs/zerolog/log"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
}

func New(port string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + port,
			Handler:      handler,
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 60 * time.Second,
		},
	}
}

func (s *Server) Run() error {
	log.Info().Msgf("Server running on port %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
