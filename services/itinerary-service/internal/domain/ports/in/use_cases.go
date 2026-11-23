package in

import (
	"context"

	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/models"
)

type ItineraryUseCases interface {
	GetConsolidatedItinerary(ctx context.Context, usuarioID, acceptLanguage string) (models.ItinerarioConsolidado, error)
	GetReservationContext(ctx context.Context, reservaID string) (models.ReservaContexto, error)
	CreateTrip(ctx context.Context, idempotencyKey string, input models.CreateTripInput) (models.Viaje, error)
	CreateReservation(ctx context.Context, idempotencyKey string, input models.CreateReservationInput) (models.Reserva, error)
	ArchiveTrip(ctx context.Context, viajeID string) (models.Viaje, error)
}
