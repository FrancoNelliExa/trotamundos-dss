package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/models"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/ports/out"
)

type fakeRepo struct {
	user          models.UsuarioResumen
	trips         []models.Viaje
	tripsByID     map[string]models.Viaje
	idempotency   map[string]out.IdempotencyRecord
	readCallCount int
}

func newFakeRepo() *fakeRepo {
	trip := models.Viaje{
		ViajeID:          "vj_5501",
		UsuarioID:        "usr_9812a3",
		NombreViaje:      "Gira Europea 2026",
		DestinoPrincipal: "Madrid, España",
		FechaInicio:      "2026-11-10",
		FechaFin:         "2026-11-25",
		Estado:           "ACTIVO",
		Reservas:         []models.Reserva{},
	}
	return &fakeRepo{
		user: models.UsuarioResumen{
			UsuarioID:  "usr_9812a3",
			Nombre:     "Nicola Satragni",
			Email:      "nicola@example.com",
			Telefono:   "+54 9 2494 123456",
			IdiomaPref: "es-AR",
			MonedaPref: "USD",
		},
		trips:       []models.Viaje{trip},
		tripsByID:   map[string]models.Viaje{trip.ViajeID: trip},
		idempotency: map[string]out.IdempotencyRecord{},
	}
}

func (f *fakeRepo) GetUser(_ context.Context, id string) (models.UsuarioResumen, error) {
	if id != f.user.UsuarioID {
		return models.UsuarioResumen{}, models.ErrNotFound
	}
	return f.user, nil
}
func (f *fakeRepo) GetActiveTripsWithReservations(_ context.Context, _ string) ([]models.Viaje, error) {
	f.readCallCount++
	return f.trips, nil
}
func (f *fakeRepo) GetReservationContext(_ context.Context, _ string) (models.ReservaContexto, error) {
	return models.ReservaContexto{}, models.ErrNotFound
}
func (f *fakeRepo) GetTripByID(_ context.Context, id string) (models.Viaje, error) {
	v, ok := f.tripsByID[id]
	if !ok {
		return models.Viaje{}, models.ErrNotFound
	}
	return v, nil
}
func (f *fakeRepo) SaveTrip(_ context.Context, t models.Viaje) error {
	f.trips = append(f.trips, t)
	f.tripsByID[t.ViajeID] = t
	return nil
}
func (f *fakeRepo) SaveReservation(_ context.Context, r models.Reserva, _ string) error {
	v := f.tripsByID[r.ViajeID]
	v.Reservas = append(v.Reservas, r)
	v.TotalReservas = len(v.Reservas)
	f.tripsByID[r.ViajeID] = v
	f.trips = []models.Viaje{v}
	return nil
}
func (f *fakeRepo) ArchiveTrip(_ context.Context, id string, _ time.Time) (models.Viaje, error) {
	v, ok := f.tripsByID[id]
	if !ok {
		return models.Viaje{}, models.ErrNotFound
	}
	v.Estado = "ARCHIVADO"
	f.tripsByID[id] = v
	return v, nil
}
func (f *fakeRepo) GetIdempotency(_ context.Context, key string) (*out.IdempotencyRecord, error) {
	rec, ok := f.idempotency[key]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}
func (f *fakeRepo) SaveIdempotency(_ context.Context, key string, rec out.IdempotencyRecord) error {
	f.idempotency[key] = rec
	return nil
}

type fakeCache struct {
	store     map[string][]byte
	failGet   bool
	deleted   []string
	lastTTL   time.Duration
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string][]byte{}} }

func (c *fakeCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	if c.failGet {
		return nil, false, errors.New("redis connection refused")
	}
	v, ok := c.store[key]
	return v, ok, nil
}
func (c *fakeCache) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	c.store[key] = val
	c.lastTTL = ttl
	return nil
}
func (c *fakeCache) Delete(_ context.Context, key string) error {
	delete(c.store, key)
	c.deleted = append(c.deleted, key)
	return nil
}

type fakePublisher struct {
	events []models.ItineraryUpdatedEvent
}

func (p *fakePublisher) PublishItineraryUpdated(_ context.Context, ev models.ItineraryUpdatedEvent) error {
	p.events = append(p.events, ev)
	return nil
}

func TestGetConsolidatedItinerary_CacheMissThenHit(t *testing.T) {
	repo := newFakeRepo()
	cache := newFakeCache()
	pub := &fakePublisher{}
	svc := NewItineraryApplicationService(repo, cache, pub)

	first, err := svc.GetConsolidatedItinerary(context.Background(), "usr_9812a3", "es-AR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.OrigenDatos != "CACHE_MISS" {
		t.Fatalf("expected CACHE_MISS, got %s", first.OrigenDatos)
	}
	if cache.lastTTL != 15*time.Minute {
		t.Fatalf("expected 15m TTL, got %v", cache.lastTTL)
	}

	second, err := svc.GetConsolidatedItinerary(context.Background(), "usr_9812a3", "es-AR")
	if err != nil {
		t.Fatalf("unexpected error on 2nd call: %v", err)
	}
	if second.OrigenDatos != "CACHE_HIT" {
		t.Fatalf("expected CACHE_HIT, got %s", second.OrigenDatos)
	}
	if repo.readCallCount != 1 {
		t.Fatalf("expected DB read count 1 (served from cache), got %d", repo.readCallCount)
	}
}

func TestGetConsolidatedItinerary_RedisDownDegradesGracefully(t *testing.T) {
	repo := newFakeRepo()
	cache := newFakeCache()
	cache.failGet = true
	svc := NewItineraryApplicationService(repo, cache, &fakePublisher{})

	res, err := svc.GetConsolidatedItinerary(context.Background(), "usr_9812a3", "")
	if err != nil {
		t.Fatalf("expected graceful degradation when Redis fails, got error: %v", err)
	}
	if res.OrigenDatos != "CACHE_MISS" {
		t.Fatalf("expected CACHE_MISS fallback, got %s", res.OrigenDatos)
	}
}

func TestCreateReservation_InvalidatesCacheAndPublishesEvent(t *testing.T) {
	repo := newFakeRepo()
	cache := newFakeCache()
	pub := &fakePublisher{}
	svc := NewItineraryApplicationService(repo, cache, pub)

	_, _ = svc.GetConsolidatedItinerary(context.Background(), "usr_9812a3", "es-AR")

	start := time.Date(2026, 11, 10, 8, 30, 0, 0, time.UTC)
	end := time.Date(2026, 11, 10, 20, 15, 0, 0, time.UTC)
	res, err := svc.CreateReservation(context.Background(), "idem-1", models.CreateReservationInput{
		ViajeID:            "vj_5501",
		TipoReserva:        "VUELO",
		Proveedor:          "Iberia",
		CodigoConfirmacion: "IB-9999",
		FechaInicioUTC:     start,
		FechaFinUTC:        end,
		ZonaHorariaOrigen:  "America/Argentina/Buenos_Aires",
		ZonaHorariaDestino: "Europe/Madrid",
		Monto:              500,
		MonedaISO:          "USD",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ReservaID == "" || !res.AgregadaManualmente {
		t.Fatalf("expected manual reservation with generated ID, got %+v", res)
	}
	if len(cache.deleted) != 1 || cache.deleted[0] != "itinerario:usr_9812a3" {
		t.Fatalf("expected cache invalidation for itinerario:usr_9812a3, got %v", cache.deleted)
	}
	if len(pub.events) != 1 || pub.events[0].UsuarioID != "usr_9812a3" {
		t.Fatalf("expected 1 itinerary.updated event, got %+v", pub.events)
	}
}

func TestCreateReservation_IdempotencyConflict(t *testing.T) {
	repo := newFakeRepo()
	svc := NewItineraryApplicationService(repo, newFakeCache(), &fakePublisher{})

	start := time.Date(2026, 11, 10, 8, 30, 0, 0, time.UTC)
	end := time.Date(2026, 11, 10, 20, 15, 0, 0, time.UTC)
	input1 := models.CreateReservationInput{
		ViajeID:            "vj_5501",
		TipoReserva:        "VUELO",
		Proveedor:          "Iberia",
		CodigoConfirmacion: "IB-1111",
		FechaInicioUTC:     start,
		FechaFinUTC:        end,
	}
	first, err := svc.CreateReservation(context.Background(), "same-key", input1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same key + same payload -> returns same reservation idempotently
	again, err := svc.CreateReservation(context.Background(), "same-key", input1)
	if err != nil {
		t.Fatalf("expected idempotent replay, got %v", err)
	}
	if again.ReservaID != first.ReservaID {
		t.Fatalf("expected same reservaId %s, got %s", first.ReservaID, again.ReservaID)
	}

	// Same key + different payload -> 409 conflict
	input2 := input1
	input2.CodigoConfirmacion = "IB-2222"
	_, err = svc.CreateReservation(context.Background(), "same-key", input2)
	if !errors.Is(err, models.ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}
}
