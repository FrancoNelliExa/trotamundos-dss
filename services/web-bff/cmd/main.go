package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/services/web-bff/internal"
)

func main() {
	port := envOr("PORT", "8080")
	cfg := internal.RouterConfig{
		ItineraryURL: envOr("ITINERARY_SERVICE_URL", "http://localhost:8081"),
		ChatbotURL:   envOr("CHATBOT_SERVICE_URL", "http://localhost:8082"),
		AgencyURL:    envOr("AGENCY_MOCK_URL", "http://localhost:8089"),
		WebDir:       envOr("WEB_DIR", "./web"),
		OpenAPIPath:  envOr("OPENAPI_PATH", "./api/openapi.yaml"),
	}

	auth, err := internal.NewJWTManager()
	if err != nil {
		slog.Error("failed to init JWT manager", "err", err)
		os.Exit(1)
	}
	handler, err := internal.NewRouter(auth, cfg)
	if err != nil {
		slog.Error("failed to init router", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("web-bff listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("web-bff error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
