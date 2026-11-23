package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/httputil"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/models"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/ports/out"
)

const cacheTTL = 15 * time.Minute

type ItineraryApplicationService struct {
	repo      out.ReservationRepositoryPort
	cache     out.CachePort
	publisher out.EventPublisherPort
}

func NewItineraryApplicationService(
	repo out.ReservationRepositoryPort,
	cache out.CachePort,
	publisher out.EventPublisherPort,
) *ItineraryApplicationService {
	return &ItineraryApplicationService{
		repo:      repo,
		cache:     cache,
		publisher: publisher,
	}
}

func cacheKey(usuarioID string) string {
	return "itinerario:" + usuarioID
}

func (s *ItineraryApplicationService) GetConsolidatedItinerary(
	ctx context.Context,
	usuarioID, acceptLanguage string,
) (models.ItinerarioConsolidado, error) {
	if usuarioID == "" {
		return models.ItinerarioConsolidado{}, fmt.Errorf("%w: usuarioId es obligatorio", models.ErrValidation)
	}

	if s.cache != nil {
		raw, hit, err := s.cache.Get(ctx, cacheKey(usuarioID))
		if err != nil {
			slog.Warn("redis cache unavailable, falling back to postgres", "err", err)
		} else if hit {
			var cached models.ItinerarioConsolidado
			if uErr := json.Unmarshal(raw, &cached); uErr == nil {
				cached.OrigenDatos = "CACHE_HIT"
				cached.Timestamp = time.Now().UTC()
				if acceptLanguage != "" {
					cached.Idioma = acceptLanguage
				}
				return cached, nil
			}
		}
	}

	user, err := s.repo.GetUser(ctx, usuarioID)
	if err != nil {
		return models.ItinerarioConsolidado{}, err
	}
	trips, err := s.repo.GetActiveTripsWithReservations(ctx, usuarioID)
	if err != nil {
		return models.ItinerarioConsolidado{}, err
	}
	if trips == nil {
		trips = []models.Viaje{}
	}

	lang := user.IdiomaPref
	if acceptLanguage != "" {
		lang = acceptLanguage
	}
	res := models.ItinerarioConsolidado{
		UsuarioID:       usuarioID,
		Timestamp:       time.Now().UTC(),
		Idioma:          lang,
		MonedaPreferida: user.MonedaPref,
		OrigenDatos:     "CACHE_MISS",
		ViajesActivos:   trips,
	}

	if s.cache != nil {
		if payload, mErr := json.Marshal(res); mErr == nil {
			if setErr := s.cache.Set(ctx, cacheKey(usuarioID), payload, cacheTTL); setErr != nil {
				slog.Warn("failed to populate redis cache", "err", setErr)
			}
		}
	}
	return res, nil
}

func (s *ItineraryApplicationService) GetReservationContext(ctx context.Context, reservaID string) (models.ReservaContexto, error) {
	return s.GetReservationContextForUser(ctx, "", reservaID)
}

func (s *ItineraryApplicationService) GetReservationContextForUser(ctx context.Context, callerUserID, reservaID string) (models.ReservaContexto, error) {
	if reservaID == "" {
		return models.ReservaContexto{}, fmt.Errorf("%w: reservaId es obligatorio", models.ErrValidation)
	}
	resCtx, err := s.repo.GetReservationContext(ctx, reservaID)
	if err != nil {
		return resCtx, err
	}
	if callerUserID != "" && resCtx.Usuario.UsuarioID != callerUserID {
		return models.ReservaContexto{}, models.ErrForbidden
	}
	return resCtx, nil
}

func (s *ItineraryApplicationService) CreateTrip(
	ctx context.Context,
	idempotencyKey string,
	input models.CreateTripInput,
) (models.Viaje, error) {
	if input.UsuarioID == "" || input.NombreViaje == "" || input.DestinoPrincipal == "" || input.FechaInicio == "" || input.FechaFin == "" {
		return models.Viaje{}, fmt.Errorf("%w: todos los campos del viaje son obligatorios", models.ErrValidation)
	}
	if input.FechaInicio > input.FechaFin {
		return models.Viaje{}, fmt.Errorf("%w: fechaInicio debe ser anterior o igual a fechaFin", models.ErrValidation)
	}

	reqHash := hashJSON(input)
	if idempotencyKey != "" {
		existing, err := s.repo.GetIdempotency(ctx, "trip:"+idempotencyKey)
		if err != nil {
			return models.Viaje{}, err
		}
		if existing != nil {
			if existing.RequestHash != reqHash {
				return models.Viaje{}, models.ErrIdempotencyConflict
			}
			var saved models.Viaje
			_ = json.Unmarshal(existing.ResponseBody, &saved)
			return saved, nil
		}
	}

	trip := models.Viaje{
		ViajeID:          httputil.NewID("vj"),
		UsuarioID:        input.UsuarioID,
		NombreViaje:      input.NombreViaje,
		DestinoPrincipal: input.DestinoPrincipal,
		FechaInicio:      input.FechaInicio,
		FechaFin:         input.FechaFin,
		Estado:           "ACTIVO",
		Reservas:         []models.Reserva{},
	}
	if err := s.repo.SaveTrip(ctx, trip); err != nil {
		return models.Viaje{}, err
	}
	if idempotencyKey != "" {
		body, _ := json.Marshal(trip)
		_ = s.repo.SaveIdempotency(ctx, "trip:"+idempotencyKey, out.IdempotencyRecord{
			RequestHash:  reqHash,
			StatusCode:   201,
			ResponseBody: body,
		})
	}
	s.invalidateAndNotify(ctx, trip.UsuarioID, trip.ViajeID, "TRIP_CREATED")
	return trip, nil
}

func (s *ItineraryApplicationService) CreateReservation(
	ctx context.Context,
	idempotencyKey string,
	input models.CreateReservationInput,
) (models.Reserva, error) {
	return s.CreateReservationForUser(ctx, "", idempotencyKey, input)
}

func (s *ItineraryApplicationService) CreateReservationForUser(
	ctx context.Context,
	callerUserID, idempotencyKey string,
	input models.CreateReservationInput,
) (models.Reserva, error) {
	if input.ViajeID == "" || input.TipoReserva == "" || input.Proveedor == "" || input.CodigoConfirmacion == "" {
		return models.Reserva{}, fmt.Errorf("%w: viajeId, tipoReserva, proveedor y codigoConfirmacion son obligatorios", models.ErrValidation)
	}
	if input.FechaInicioUTC.IsZero() || input.FechaFinUTC.IsZero() || !input.FechaInicioUTC.Before(input.FechaFinUTC) {
		return models.Reserva{}, fmt.Errorf("%w: fechaInicioUtc debe ser una fecha ISO 8601 válida y anterior a fechaFinUtc", models.ErrValidation)
	}

	trip, err := s.repo.GetTripByID(ctx, input.ViajeID)
	if err != nil {
		return models.Reserva{}, err
	}
	if callerUserID != "" && trip.UsuarioID != callerUserID {
		return models.Reserva{}, models.ErrForbidden
	}

	reqHash := hashJSON(input)
	if idempotencyKey != "" {
		existing, err := s.repo.GetIdempotency(ctx, "res:"+idempotencyKey)
		if err != nil {
			return models.Reserva{}, err
		}
		if existing != nil {
			if existing.RequestHash != reqHash {
				return models.Reserva{}, models.ErrIdempotencyConflict
			}
			var saved models.Reserva
			_ = json.Unmarshal(existing.ResponseBody, &saved)
			return saved, nil
		}
	}

	tzOrig := input.ZonaHorariaOrigen
	if tzOrig == "" {
		tzOrig = "UTC"
	}
	tzDest := input.ZonaHorariaDestino
	if tzDest == "" {
		tzDest = tzOrig
	}
	moneda := input.MonedaISO
	if moneda == "" {
		moneda = "USD"
	}
	detalles := input.Detalles
	if detalles == nil {
		detalles = map[string]any{}
	}

	res := models.Reserva{
		ReservaID:           httputil.NewID("res"),
		ViajeID:             input.ViajeID,
		TipoReserva:         input.TipoReserva,
		Proveedor:           input.Proveedor,
		CodigoConfirmacion:  input.CodigoConfirmacion,
		Estado:              "CONFIRMADA",
		FechaInicioUTC:      input.FechaInicioUTC.UTC(),
		FechaFinUTC:         input.FechaFinUTC.UTC(),
		ZonaHorariaOrigen:   tzOrig,
		ZonaHorariaDestino:  tzDest,
		Monto:               input.Monto,
		MonedaISO:           moneda,
		AgregadaManualmente: true,
		Detalles:            detalles,
	}

	if err := s.repo.SaveReservation(ctx, res, idempotencyKey); err != nil {
		return models.Reserva{}, err
	}
	if idempotencyKey != "" {
		body, _ := json.Marshal(res)
		_ = s.repo.SaveIdempotency(ctx, "res:"+idempotencyKey, out.IdempotencyRecord{
			RequestHash:  reqHash,
			StatusCode:   201,
			ResponseBody: body,
		})
	}
	s.invalidateAndNotify(ctx, trip.UsuarioID, trip.ViajeID, "RESERVATION_CREATED")
	return res, nil
}

func (s *ItineraryApplicationService) ArchiveTrip(ctx context.Context, viajeID string) (models.Viaje, error) {
	return s.ArchiveTripForUser(ctx, "", viajeID)
}

func (s *ItineraryApplicationService) ArchiveTripForUser(ctx context.Context, callerUserID, viajeID string) (models.Viaje, error) {
	if viajeID == "" {
		return models.Viaje{}, fmt.Errorf("%w: viajeId es obligatorio", models.ErrValidation)
	}
	if callerUserID != "" {
		existing, err := s.repo.GetTripByID(ctx, viajeID)
		if err != nil {
			return models.Viaje{}, err
		}
		if existing.UsuarioID != callerUserID {
			return models.Viaje{}, models.ErrForbidden
		}
	}
	trip, err := s.repo.ArchiveTrip(ctx, viajeID, time.Now().UTC())
	if err != nil {
		return models.Viaje{}, err
	}
	s.invalidateAndNotify(ctx, trip.UsuarioID, trip.ViajeID, "TRIP_ARCHIVED")
	return trip, nil
}

func (s *ItineraryApplicationService) invalidateAndNotify(ctx context.Context, usuarioID, viajeID, action string) {
	if s.cache != nil {
		if err := s.cache.Delete(ctx, cacheKey(usuarioID)); err != nil {
			slog.Warn("failed to invalidate redis cache", "usuarioId", usuarioID, "err", err)
		}
	}
	if s.publisher != nil {
		ev := models.ItineraryUpdatedEvent{
			EventID:   httputil.NewID("evt"),
			UsuarioID: usuarioID,
			ViajeID:   viajeID,
			Action:    action,
			Timestamp: time.Now().UTC(),
		}
		if err := s.publisher.PublishItineraryUpdated(ctx, ev); err != nil {
			slog.Warn("failed to publish itinerary.updated event", "err", err)
		}
	}
}

func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
