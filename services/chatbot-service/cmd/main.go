package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/httputil"
	"github.com/FrancoNelliExa/trotamundos-dss/pkg/resilience"
	"github.com/FrancoNelliExa/trotamundos-dss/services/chatbot-service/internal/infrastructure/adapters"
	"github.com/FrancoNelliExa/trotamundos-dss/services/chatbot-service/internal/infrastructure/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
	kafkago "github.com/segmentio/kafka-go"
)

func main() {
	port := envOr("PORT", "8082")
	dbURL := envOr("DATABASE_URL", "postgres://trotamundos:trotamundos@localhost:5433/trotamundos?sslmode=disable")
	itineraryURL := envOr("ITINERARY_SERVICE_URL", "http://localhost:8081")
	agencyURL := envOr("AGENCY_MOCK_URL", "http://localhost:8089")
	serviceToken := envOr("SERVICE_TOKEN", "trotamundos-internal-service-token")
	kafkaBroker := envOr("KAFKA_BROKER", "localhost:19092")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// ADR-014: apertura tras 5 fallos consecutivos en 30s, Half-Open tras 30s
	cb := resilience.NewCircuitBreaker(resilience.Config{
		FailureThreshold: 5,
		FailureWindow:    30 * time.Second,
		ResetTimeout:     30 * time.Second,
	})

	repo := outbox.NewPGOutboxRepository(pool)
	itinClient := adapters.NewItineraryClient(itineraryURL)
	agencyClient := adapters.NewAgencyWebhookClient(agencyURL, serviceToken, cb)

	kafkaWriter := &kafkago.Writer{
		Addr:                   kafkago.TCP(kafkaBroker),
		Topic:                  "chatbot.events",
		Balancer:               &kafkago.Hash{},
		AllowAutoTopicCreation: true,
		Async:                  true,
		WriteTimeout:           2 * time.Second,
		BatchTimeout:           20 * time.Millisecond,
	}
	defer kafkaWriter.Close()

	ctrl := adapters.NewRestChatbotController(repo, itinClient, agencyClient, kafkaWriter)

	worker := outbox.NewWorker(repo, agencyClient, func(c context.Context, tk outbox.TicketRecord, _ string) {
		tk.Estado = "ENTREGADO"
		ctrl.PublishTicketEvent(c, tk, "TICKET_ENTREGADO_OUTBOX")
	})
	go worker.Run(ctx)

	mux := http.NewServeMux()
	ctrl.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           httputil.WithCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("chatbot-service listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("chatbot-service failed", "err", err)
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
