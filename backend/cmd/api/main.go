package main

import (
	_ "chatnip/docs"
	"chatnip/internal/kit/auth"
	"chatnip/internal/kit/config"
	"chatnip/internal/kit/database"
	"chatnip/internal/kit/health"
	"chatnip/internal/kit/middleware"
	"chatnip/internal/kit/queue"
	"chatnip/internal/kit/server"
	"chatnip/internal/modules/whatsapp/delivery/http/handler"
	"chatnip/internal/modules/whatsapp/infrastructure"
	"chatnip/internal/modules/whatsapp/infrastructure/persistence"
	waQueue "chatnip/internal/modules/whatsapp/infrastructure/queue"
	waWeb "chatnip/internal/modules/whatsapp/infrastructure/web"
	"chatnip/internal/modules/whatsapp/service"
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// @title           ChatNip WhatsApp Gateway API
// @version         1.0
// @description     Enterprise-Grade Multi-Tenant WhatsApp Gateway & Event Dispatcher.
// @termsOfService  http://swagger.io/terms/

// @contact.name   ChatNip Engineering
// @contact.url    https://github.com/daviroodrigues/ChatNip

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @BasePath  /

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description Chave de API de cliente para envio e operações de mensagens (formato cn_live_*)

// @securityDefinitions.apikey InternalKeyAuth
// @in header
// @name X-Internal-Key
// @description Chave interna administrativa mestre para gerenciamento de credenciais e instâncias

func main() {

	cfg := config.Load()
	zLogger := setupLogger(cfg.Environment)

	dbConn, err := database.NewSQLiteConnection(cfg.DBPath)
	if err != nil {
		zLogger.Fatal().Err(err).Msg("Fatal database connection error")
	}
	defer dbConn.Close()

	rmqConfig := queue.Config{
		URL:          cfg.RabbitURL,
		ExchangeName: "whatsapp_events",
		ExchangeType: "topic",
	}
	rabbitAdapter, err := queue.NewRabbitMQAdapter(rmqConfig)
	if err != nil {
		zLogger.Fatal().Err(err).Msg("Could not connect to RabbitMQ")
	}
	defer rabbitAdapter.Close()

	waRepo := persistence.NewSQLiteWhatsAppRepository(dbConn, cfg.EncryptionKey)

	waContainer, err := persistence.NewWhatsMeowContainer(cfg.DBPath)
	if err != nil {
		zLogger.Fatal().Err(err).Msg("Failed to initialize whatsmeow store")
	}

	waProducer := waQueue.NewWhatsAppProducer(rabbitAdapter)
	clientRepo := infrastructure.NewClientRepository(waContainer, cfg, zLogger)
	downloader := &waWeb.HTTPFileDownloader{}
	mediaIngestRepo := persistence.NewSQLiteMediaIngestRepository(dbConn)
	mediaIngestSvc := service.NewMediaIngestService(
		mediaIngestRepo,
		clientRepo,
		waProducer,
		cfg.BackendURL,
		cfg.BackendInternalKey,
		cfg.MediaTempDir,
		zLogger,
	)

	msgHandler := service.NewMessageHandler(zLogger, waProducer, mediaIngestSvc)
	pairHandler := service.NewPairSuccessHandler(waRepo, zLogger)
	historyHandler := service.NewHistorySyncHandler(zLogger, waProducer)
	receiptHandler := service.NewReceiptHandler(zLogger, waProducer)
	presenceHandler := service.NewChatPresenceHandler(zLogger, waProducer)
	callHandler := service.NewCallOfferHandler(zLogger, waProducer)
	callTerminateHandler := service.NewCallTerminateHandler(zLogger, waProducer)
	undecryptableHandler := service.NewUndecryptableMessageHandler(zLogger, waProducer)
	groupInfoHandler := service.NewGroupInfoHandler(zLogger, waProducer)
	dispatcher := service.NewEventDispatcher(
		msgHandler,
		pairHandler,
		historyHandler,
		receiptHandler,
		presenceHandler,
		callHandler,
		callTerminateHandler,
		undecryptableHandler,
		groupInfoHandler,
	)

	waService := service.NewWhatsAppService(
		waRepo,
		clientRepo,
		dispatcher,
		cfg,
		zLogger,
		waProducer,
		downloader,
	)

	go waService.RestoreSessions()

	waWorker := waQueue.NewWhatsAppWorker(waService, rabbitAdapter, zLogger)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	mediaIngestSvc.StartRetryWorker(workerCtx)

	waWorker.StartWithPrefetch(workerCtx, "whatsapp_fast_lane", 15, 10, true)
	waWorker.StartWithPrefetch(workerCtx, "whatsapp_standard_lane", 3, 1, false)
	waWorker.StartGroupWorker(workerCtx, "whatsapp_group_create", 2)

	waHandler := handler.NewWhatsAppHandler(waService, zLogger)
	healthHandler := health.NewHandler(dbConn)

	authStore := auth.NewSQLStore(dbConn)
	authHandler := auth.NewHandler(authStore, zLogger)

	r := chi.NewRouter()

	r.Use(middleware.Logger(zLogger))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "X-API-Key"},
		AllowCredentials: true,
	}))

	r.Get("/health", healthHandler.Check)

	// Swagger documentation
	r.Get("/swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
	})
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
	})
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// Auth & API Key generation (requires X-Internal-Key)
	authHandler.RegisterRoutes(r, cfg.APIVersion, cfg.InternalKey)

	// WhatsApp API routes (authenticated via X-API-Key or Bearer token)
	waHandler.RegisterRoutes(r, cfg, authStore)

	srv := server.New(cfg.Port, r)

	go func() {
		if err := srv.Run(); err != nil && err != http.ErrServerClosed {
			zLogger.Fatal().Err(err).Msg("Server failed")
		}
	}()
	zLogger.Info().Msgf("Server started on port %s", cfg.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	zLogger.Info().Msg("Server is shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zLogger.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	waService.Shutdown()

	zLogger.Info().Msg("Server exited properly")
}

func setupLogger(env string) zerolog.Logger {
	var logOutput io.Writer = os.Stdout
	logLevel := zerolog.InfoLevel

	if env == "development" {
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		output.FormatLevel = func(i interface{}) string {
			if ll, ok := i.(string); ok {
				return strings.ToUpper(ll)[0:3]
			}
			return "UNK"
		}
		logOutput = output
		logLevel = zerolog.DebugLevel
	}

	zerolog.SetGlobalLevel(logLevel)
	return zerolog.New(logOutput).With().Timestamp().Logger()
}
