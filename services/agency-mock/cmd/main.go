package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/httputil"
)

type AgencyServer struct {
	mu      sync.Mutex
	mode    string
	tickets map[string]map[string]any
	order   []string
}

func NewAgencyServer() *AgencyServer {
	return &AgencyServer{
		mode:    "healthy",
		tickets: map[string]map[string]any{},
	}
}

func (s *AgencyServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "agency-mock"})
	})
	mux.HandleFunc("GET /mock/mode", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		mode := s.mode
		total := len(s.order)
		s.mu.Unlock()
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"mode": mode, "ticketsRecibidos": total})
	})
	mux.HandleFunc("POST /mock/mode", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Payload inválido.", err.Error())
			return
		}
		if body.Mode != "healthy" && body.Mode != "slow" && body.Mode != "error500" {
			httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Modo inválido.", "Use healthy, slow o error500.")
			return
		}
		s.mu.Lock()
		s.mode = body.Mode
		s.mu.Unlock()
		slog.Info("agency-mock mode updated", "mode", body.Mode)
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"mode": body.Mode})
	})
	mux.HandleFunc("GET /mock/tickets", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		items := make([]map[string]any, 0, len(s.order))
		for _, k := range s.order {
			items = append(items, s.tickets[k])
		}
		s.mu.Unlock()
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"total": len(items), "tickets": items})
	})
	mux.HandleFunc("POST /api/v1/agencia/derivacion", s.handleDerivacion)
	return httputil.WithCORS(mux)
}

func (s *AgencyServer) handleDerivacion(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()

	if mode == "slow" {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
			return
		}
	}
	if mode == "error500" {
		httputil.WriteProblem(w, r, http.StatusInternalServerError, "AGENCY_UNAVAILABLE", "Agencia externa indisponible.", "Falla simulada en mesa de ayuda de la agencia.")
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httputil.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Payload inválido.", err.Error())
		return
	}

	ticketID, _ := payload["ticketDerivacion"].(string)
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		idemKey, _ = payload["idempotencyKey"].(string)
	}
	if idemKey == "" {
		idemKey = ticketID
	}

	s.mu.Lock()
	if _, exists := s.tickets[idemKey]; !exists {
		s.tickets[idemKey] = payload
		s.order = append(s.order, idemKey)
	}
	s.mu.Unlock()

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"ticketDerivacion": ticketID,
		"estado":           "RECIBIDO",
		"agenteAsignado":   "Operador_Agencia_04",
		"mensaje":          "Ticket registrado en la mesa de ayuda externa con prioridad Alta.",
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
	}
	srv := NewAgencyServer()
	slog.Info("agency-mock listening", "port", port)
	if err := http.ListenAndServe(":"+port, srv.Handler()); err != nil {
		slog.Error("agency-mock failed", "err", err)
		os.Exit(1)
	}
}
