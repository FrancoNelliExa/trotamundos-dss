package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/resilience"
	"github.com/FrancoNelliExa/trotamundos-dss/services/chatbot-service/internal/infrastructure/outbox"
)

type memChatRepo struct {
	savedTickets []outbox.TicketRecord
}

func (m *memChatRepo) EnsureSession(_ context.Context, _, _ string) error { return nil }
func (m *memChatRepo) SaveMessage(_ context.Context, _, _, _, _ string) error { return nil }
func (m *memChatRepo) MarkSessionEscalated(_ context.Context, _ string) error { return nil }
func (m *memChatRepo) SaveTicket(_ context.Context, tk outbox.TicketRecord) error {
	m.savedTickets = append(m.savedTickets, tk)
	return nil
}
func (m *memChatRepo) ListTicketsByUser(_ context.Context, _ string) ([]outbox.TicketRecord, error) {
	return m.savedTickets, nil
}

func TestChatbot_UnknownReservaIDClearsFKAndCrossUserReturns403(t *testing.T) {
	itinMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/reservas/res_victim" && r.URL.Query().Get("usuarioId") == "usr_attacker" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer itinMock.Close()

	agencyMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"estado":"RECIBIDO","agenteAsignado":"Op1"}`))
	}))
	defer agencyMock.Close()

	repo := &memChatRepo{}
	cb := resilience.NewCircuitBreaker(resilience.Config{FailureThreshold: 5, FailureWindow: 30 * time.Second, ResetTimeout: 30 * time.Second})
	ctrl := NewRestChatbotController(repo, NewItineraryClient(itinMock.URL), NewAgencyWebhookClient(agencyMock.URL, "tok", cb), nil)

	mux := http.NewServeMux()
	ctrl.RegisterRoutes(mux)

	// 1. Cross-user reservaId -> must return 403 Forbidden
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/chatbot/consulta", bytes.NewBufferString(`{"usuarioId":"usr_attacker","reservaId":"res_victim","mensajeUsuario":"Estado de vuelo"}`))
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)
	if w1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-user reservaId, got %d (%s)", w1.Code, w1.Body.String())
	}

	// 2. Unknown reservaId on escalation -> succeeds (200) and stores empty ReservaID (NULL in DB, no FK 500)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/chatbot/consulta", bytes.NewBufferString(`{"usuarioId":"usr_9812a3","reservaId":"res_inexistente","mensajeUsuario":"Quiero reembolso total"}`))
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on unknown reservaId escalation, got %d (%s)", w2.Code, w2.Body.String())
	}
	if len(repo.savedTickets) != 1 || repo.savedTickets[0].ReservaID != "" {
		t.Fatalf("expected saved ticket with empty ReservaID (NULL FK), got %+v", repo.savedTickets)
	}
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["requiereDerivacion"] != true {
		t.Fatalf("expected requiereDerivacion=true, got %v", resp)
	}
}
