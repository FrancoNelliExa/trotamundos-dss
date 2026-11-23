package out

import (
	"context"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/models"
)

type IdempotencyRecord struct {
	RequestHash  string
	StatusCode   int
	ResponseBody []byte
}

type ReservationRepositoryPort interface {
	GetUser(ctx context.Context, usuarioID string) (models.UsuarioResumen, error)
	GetActiveTripsWithReservations(ctx context.Context, usuarioID string) ([]models.Viaje, error)
	GetReservationContext(ctx context.Context, reservaID string) (models.ReservaContexto, error)
	GetTripByID(ctx context.Context, viajeID string) (models.Viaje, error)
	SaveTrip(ctx context.Context, trip models.Viaje) error
	SaveReservation(ctx context.Context, res models.Reserva, idempotencyKey string) error
	ArchiveTrip(ctx context.Context, viajeID string, archivedAt time.Time) (models.Viaje, error)
	GetIdempotency(ctx context.Context, key string) (*IdempotencyRecord, error)
	SaveIdempotency(ctx context.Context, key string, rec IdempotencyRecord) error
}

type CachePort interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

type EventPublisherPort interface {
	PublishItineraryUpdated(ctx context.Context, event models.ItineraryUpdatedEvent) error
}
