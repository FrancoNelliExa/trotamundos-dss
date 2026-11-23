package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/httputil"
	"github.com/FrancoNelliExa/trotamundos-dss/pkg/resilience"
	"github.com/FrancoNelliExa/trotamundos-dss/services/chatbot-service/internal/domain"
	"github.com/FrancoNelliExa/trotamundos-dss/services/chatbot-service/internal/infrastructure/outbox"
	kafkago "github.com/segmentio/kafka-go"
)

const defaultAgencyName = "Agencia Viajes Tandil"

type ConsultaRequest struct {
	UsuarioID      string `json:"usuarioId"`
	ReservaID      string `json:"reservaId"`
	MensajeUsuario string `json:"mensajeUsuario"`
}

type RestChatbotController struct {
	repo            *outbox.PGOutboxRepository
	itineraryClient *ItineraryClient
	agencyClient    *AgencyWebhookClient
	kafkaWriter     *kafkago.Writer
}

func NewRestChatbotController(
	repo *outbox.PGOutboxRepository,
	itineraryClient *ItineraryClient,
	agencyClient *AgencyWebhookClient,
	kafkaWriter *kafkago.Writer,
) *RestChatbotController {
	return &RestChatbotController{
		repo:            repo,
		itineraryClient: itineraryClient,
		agencyClient:    agencyClient,
		kafkaWriter:     kafkaWriter,
	}
}

func (c *RestChatbotController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "chatbot-service"})
	})
	mux.HandleFunc("POST /api/v1/chatbot/consulta", c.handleConsulta)
	mux.HandleFunc("GET /api/v1/chatbot/tickets/{usuarioId}", c.handleListTickets)
	mux.HandleFunc("GET /api/v1/chatbot/circuit-status", c.handleCircuitStatus)
}

func (c *RestChatbotController) handleConsulta(w http.ResponseWriter, r *http.Request) {
	var req ConsultaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Payload JSON inválido.", err.Error())
		return
	}
	req.MensajeUsuario = strings.TrimSpace(req.MensajeUsuario)
	if req.UsuarioID == "" || req.MensajeUsuario == "" {
		httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Campos obligatorios ausentes.", "usuarioId y mensajeUsuario son obligatorios.")
		return
	}
	if len(req.MensajeUsuario) > 2000 {
		httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Mensaje demasiado largo.", "mensajeUsuario no puede superar 2000 caracteres.")
		return
	}

	ctx := r.Context()
	sesionID := "ses_" + req.UsuarioID
	if err := c.repo.EnsureSession(ctx, sesionID, req.UsuarioID); err != nil {
		httputil.WriteProblem(w, r, http.StatusInternalServerError, "SESSION_ERROR", "No se pudo iniciar sesión de chat.", err.Error())
		return
	}
	_ = c.repo.SaveMessage(ctx, httputil.NewID("msg"), sesionID, "USUARIO", req.MensajeUsuario)

	resCtx, err := c.itineraryClient.FetchReservationContext(ctx, req.UsuarioID, req.ReservaID)
	if err != nil {
		slog.Warn("could not fetch reservation context, continuing gracefully", "err", err)
	}
	var resInfo *domain.ReservationInfo
	if resCtx != nil {
		resInfo = &resCtx.Reserva
	}

	result := domain.ClassifyAndRespond(req.MensajeUsuario, resInfo)
	consultaID := httputil.NewID("cst")

	if !result.RequiresEscalation {
		_ = c.repo.SaveMessage(ctx, httputil.NewID("msg"), sesionID, "BOT", result.Reply)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"consultaId":         consultaID,
			"respuestaBot":       result.Reply,
			"requiereDerivacion": false,
			"agenciaDestino":     nil,
			"circuitState":       c.agencyClient.CircuitStatus().State,
		})
		return
	}

	ticketID := "TCK-" + strings.ToUpper(httputil.NewID("t")[2:7])
	idemKey := httputil.NewID("idem")

	userPayload := map[string]any{
		"usuarioId": req.UsuarioID,
		"nombre":    "Viajero Trotamundos",
		"email":     "",
		"telefono":  "",
	}
	reservaPayload := map[string]any{
		"reservaId": req.ReservaID,
	}
	reservaID := req.ReservaID
	if resCtx != nil {
		userPayload = map[string]any{
			"usuarioId": resCtx.Usuario.UsuarioID,
			"nombre":    resCtx.Usuario.Nombre,
			"email":     resCtx.Usuario.Email,
			"telefono":  resCtx.Usuario.Telefono,
		}
		reservaID = resCtx.Reserva.ReservaID
		reservaPayload = map[string]any{
			"reservaId":          resCtx.Reserva.ReservaID,
			"proveedor":          resCtx.Reserva.Proveedor,
			"codigoConfirmacion": resCtx.Reserva.CodigoConfirmacion,
			"tipo":               resCtx.Reserva.TipoReserva,
		}
	}

	agencyPayload, _ := json.Marshal(map[string]any{
		"ticketDerivacion": ticketID,
		"idempotencyKey":   idemKey,
		"usuario":          userPayload,
		"reservaAfectada":  reservaPayload,
		"motivoDerivacion": result.EscalationReason,
		"historialConversacion": []map[string]string{
			{"emisor": "USUARIO", "texto": req.MensajeUsuario},
			{"emisor": "BOT", "texto": result.Reply},
		},
	})

	agente, delivErr := c.agencyClient.DeliverTicket(ctx, idemKey, agencyPayload)
	now := time.Now().UTC()
	tk := outbox.TicketRecord{
		TicketID:            ticketID,
		SesionID:            sesionID,
		UsuarioID:           req.UsuarioID,
		ReservaID:           reservaID,
		AgenciaDestino:      defaultAgencyName,
		MotivoDerivacion:    result.EscalationReason,
		PayloadJSON:         agencyPayload,
		IdempotencyKey:      idemKey,
		ProximoReintentoUTC: now.Add(5 * time.Second),
	}

	replyText := result.Reply
	if delivErr == nil {
		tk.Estado = "ENTREGADO"
		tk.AgenteAsignado = agente
		tk.EntregadoEn = &now
		replyText = "Tu solicitud requiere autorización especial. Transferimos exitosamente tu caso a tu agencia de viajes preferida (Ticket " + ticketID + ")."
	} else {
		tk.Estado = "PENDIENTE"
		tk.Intentos = 1
		tk.UltimoError = delivErr.Error()
		if errors.Is(delivErr, resilience.ErrCircuitOpen) {
			replyText = "Registramos tu pedido urgente en cola de contingencia (Ticket " + ticketID + "). El circuito hacia tu agencia está abierto para proteger la plataforma; reenviaremos tu ticket automáticamente."
		} else {
			replyText = "Registramos tu pedido urgente (Ticket " + ticketID + "). Tu agencia se comunicará a la brevedad apenas recupere conectividad."
		}
	}

	if err := c.repo.SaveTicket(ctx, tk); err != nil {
		httputil.WriteProblem(w, r, http.StatusInternalServerError, "OUTBOX_ERROR", "No se pudo registrar el ticket de derivación.", err.Error())
		return
	}
	_ = c.repo.MarkSessionEscalated(ctx, sesionID)
	_ = c.repo.SaveMessage(ctx, httputil.NewID("msg"), sesionID, "BOT", replyText)
	c.PublishTicketEvent(ctx, tk, "TICKET_"+tk.Estado)

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"consultaId":              consultaID,
		"respuestaBot":            replyText,
		"requiereDerivacion":      true,
		"agenciaDestino":          defaultAgencyName,
		"ticketDerivacion":        ticketID,
		"tiempoEstimadoRespuesta": "15 minutos",
		"estadoTicket":            tk.Estado,
		"circuitState":            c.agencyClient.CircuitStatus().State,
	})
}

func (c *RestChatbotController) handleListTickets(w http.ResponseWriter, r *http.Request) {
	usuarioID := r.PathValue("usuarioId")
	tickets, err := c.repo.ListTicketsByUser(r.Context(), usuarioID)
	if err != nil {
		httputil.WriteProblem(w, r, http.StatusInternalServerError, "DB_ERROR", "Error consultando tickets.", err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"usuarioId": usuarioID,
		"tickets":   tickets,
	})
}

func (c *RestChatbotController) handleCircuitStatus(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, c.agencyClient.CircuitStatus())
}

func (c *RestChatbotController) PublishTicketEvent(ctx context.Context, tk outbox.TicketRecord, eventType string) {
	if c.kafkaWriter == nil {
		return
	}
	ev, _ := json.Marshal(map[string]any{
		"eventType":      eventType,
		"ticketId":       tk.TicketID,
		"usuarioId":      tk.UsuarioID,
		"estado":         tk.Estado,
		"agenciaDestino": tk.AgenciaDestino,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
	if err := c.kafkaWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(tk.UsuarioID),
		Value: ev,
	}); err != nil {
		slog.Warn("failed to publish chatbot event to kafka", "err", err)
	}
}
