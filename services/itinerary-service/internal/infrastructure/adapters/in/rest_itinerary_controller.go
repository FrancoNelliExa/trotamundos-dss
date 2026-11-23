package in

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/httputil"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/models"
	portin "github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/ports/in"
)

type RestItineraryController struct {
	useCases portin.ItineraryUseCases
}

func NewRestItineraryController(uc portin.ItineraryUseCases) *RestItineraryController {
	return &RestItineraryController{useCases: uc}
}

func (c *RestItineraryController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "itinerary-service"})
	})
	mux.HandleFunc("GET /api/v1/itinerarios/{usuarioId}", c.handleGetItinerary)
	mux.HandleFunc("GET /api/v1/reservas/{reservaId}", c.handleGetReservationContext)
	mux.HandleFunc("POST /api/v1/viajes", c.handleCreateTrip)
	mux.HandleFunc("POST /api/v1/reservas", c.handleCreateReservation)
	mux.HandleFunc("PUT /api/v1/viajes/{viajeId}/archivar", c.handleArchiveTrip)
}

func (c *RestItineraryController) handleGetItinerary(w http.ResponseWriter, r *http.Request) {
	usuarioID := r.PathValue("usuarioId")
	lang := r.Header.Get("Accept-Language")
	res, err := c.useCases.GetConsolidatedItinerary(r.Context(), usuarioID, lang)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, res)
}

func (c *RestItineraryController) handleGetReservationContext(w http.ResponseWriter, r *http.Request) {
	reservaID := r.PathValue("reservaId")
	callerUserID := r.URL.Query().Get("usuarioId")
	if callerUserID == "" {
		callerUserID = r.Header.Get("X-Authenticated-User")
	}
	ctxData, err := c.useCases.GetReservationContextForUser(r.Context(), callerUserID, reservaID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, ctxData)
}

func (c *RestItineraryController) handleCreateTrip(w http.ResponseWriter, r *http.Request) {
	var input models.CreateTripInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Payload JSON inválido.", err.Error())
		return
	}
	trip, err := c.useCases.CreateTrip(r.Context(), r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"viajeId":     trip.ViajeID,
		"nombreViaje": trip.NombreViaje,
		"estado":      trip.Estado,
		"creadoEn":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (c *RestItineraryController) handleCreateReservation(w http.ResponseWriter, r *http.Request) {
	var input models.CreateReservationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Payload JSON inválido.", err.Error())
		return
	}
	callerUserID := r.Header.Get("X-Authenticated-User")
	res, err := c.useCases.CreateReservationForUser(r.Context(), callerUserID, r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"reservaId":           res.ReservaID,
		"viajeId":             res.ViajeID,
		"estado":              res.Estado,
		"agregadaManualmente": res.AgregadaManualmente,
		"mensaje":             "Reserva asociada exitosamente al viaje " + res.ViajeID + ".",
	})
}

func (c *RestItineraryController) handleArchiveTrip(w http.ResponseWriter, r *http.Request) {
	viajeID := r.PathValue("viajeId")
	callerUserID := r.Header.Get("X-Authenticated-User")
	trip, err := c.useCases.ArchiveTripForUser(r.Context(), callerUserID, viajeID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"viajeId":     trip.ViajeID,
		"estado":      trip.Estado,
		"archivadoEn": time.Now().UTC().Format(time.RFC3339),
		"mensaje":     "El viaje fue archivado y removido del panel principal.",
	})
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, models.ErrValidation):
		httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Los parámetros enviados no superaron las reglas de validación.", err.Error())
	case errors.Is(err, models.ErrForbidden):
		httputil.WriteProblem(w, r, http.StatusForbidden, "FORBIDDEN", "Acceso denegado.", "No tiene permisos sobre el viaje o reserva solicitada.")
	case errors.Is(err, models.ErrNotFound):
		httputil.WriteProblem(w, r, http.StatusNotFound, "NOT_FOUND", "Recurso no encontrado.", err.Error())
	case errors.Is(err, models.ErrIdempotencyConflict):
		httputil.WriteProblem(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Conflicto de Idempotency-Key.", err.Error())
	default:
		httputil.WriteProblem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor.", err.Error())
	}
}
