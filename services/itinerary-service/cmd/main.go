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
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/application/services"
	adapterin "github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/infrastructure/adapters/in"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/infrastructure/adapters/out/kafka"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/infrastructure/adapters/out/postgres"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/infrastructure/adapters/out/redis"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	port := envOr("PORT", "8081")
	dbURL := envOr("DATABASE_URL", "postgres://trotamundos:trotamundos@localhost:5433/trotamundos?sslmode=disable")
	redisAddr := envOr("REDIS_ADDR", "localhost:6380")
	kafkaBroker := envOr("KAFKA_BROKER", "localhost:19092")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := postgres.NewPGRepository(pool)
	cache := redis.NewRedisCache(redisAddr)
	defer cache.Close()
	pub := kafka.NewKafkaPublisher(kafkaBroker, "itinerary.updated")
	defer pub.Close()

	appSvc := services.NewItineraryApplicationService(repo, cache, pub)
	ctrl := adapterin.NewRestItineraryController(appSvc)

	mux := http.NewServeMux()
	ctrl.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           httputil.WithCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("itinerary-service listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
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
