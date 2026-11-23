package domain

import (
	"strings"
	"testing"
)

func TestClassifyIntent_Level1Questions(t *testing.T) {
	ctx := &ReservationInfo{
		ReservaID:          "res_8819",
		TipoReserva:        "VUELO",
		Proveedor:          "Iberia",
		CodigoConfirmacion: "IB-7412A",
		Estado:             "CONFIRMADA",
		FechaInicioUTC:     "2026-11-10T08:30:00Z",
		ZonaHorariaOrigen:  "America/Argentina/Buenos_Aires",
		Detalles: map[string]any{
			"origen":         "EZE",
			"destino":        "MAD",
			"numeroVuelo":    "IB6844",
			"puertaEmbarque": "B12",
		},
	}

	cases := []struct {
		msg      string
		contains string
	}{
		{"¿Puedo agregar equipaje de mano a mi vuelo de Iberia?", "equipaje"},
		{"¿Cuál es el estado de mi vuelo y la puerta de embarque?", "CONFIRMADA"},
		{"¿A qué hora es el check-in de mi reserva?", "check-in"},
	}

	for _, tc := range cases {
		res := ClassifyAndRespond(tc.msg, ctx)
		if res.RequiresEscalation {
			t.Fatalf("expected Level 1 (no escalation) for %q, got escalation (%s)", tc.msg, res.EscalationReason)
		}
		if !strings.Contains(strings.ToLower(res.Reply), strings.ToLower(tc.contains)) {
			t.Fatalf("expected reply for %q to mention %q, got %q", tc.msg, tc.contains, res.Reply)
		}
	}
}

func TestClassifyIntent_ComplexEscalation(t *testing.T) {
	ctx := &ReservationInfo{
		ReservaID:   "res_8819",
		TipoReserva: "VUELO",
		Proveedor:   "Iberia",
		Estado:      "CONFIRMADA",
	}

	prompts := []string{
		"Quiero cancelación con reembolso total a mi tarjeta",
		"Mi vuelo fue cancelado por fuerza mayor y necesito reprogramar urgente",
		"Necesito hablar con un agente humano de mi agencia",
	}

	for _, p := range prompts {
		res := ClassifyAndRespond(p, ctx)
		if !res.RequiresEscalation {
			t.Fatalf("expected escalation for %q, got Level 1 reply %q", p, res.Reply)
		}
		if res.EscalationReason == "" {
			t.Fatalf("expected non-empty escalation reason for %q", p)
		}
	}
}

func TestClassifyIntent_NoReservationContextDoesNotPanic(t *testing.T) {
	res := ClassifyAndRespond("¿Cuál es el estado de mi vuelo?", nil)
	if res.RequiresEscalation {
		t.Fatalf("expected graceful Level 1 reply when reservation context is nil")
	}
	if res.Reply == "" {
		t.Fatalf("expected non-empty reply when reservation context is nil")
	}
}
