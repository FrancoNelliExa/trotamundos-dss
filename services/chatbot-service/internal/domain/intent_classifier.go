package domain

import (
	"fmt"
	"strings"
)

type ReservationInfo struct {
	ReservaID          string         `json:"reservaId"`
	ViajeID            string         `json:"viajeId"`
	TipoReserva        string         `json:"tipoReserva"`
	Proveedor          string         `json:"proveedor"`
	CodigoConfirmacion string         `json:"codigoConfirmacion"`
	Estado             string         `json:"estado"`
	FechaInicioUTC     string         `json:"fechaInicioUtc"`
	FechaFinUTC        string         `json:"fechaFinUtc"`
	ZonaHorariaOrigen  string         `json:"zonaHorariaOrigen"`
	ZonaHorariaDestino string         `json:"zonaHorariaDestino"`
	Detalles           map[string]any `json:"detalles,omitempty"`
}

type ClassificationResult struct {
	Reply              string
	RequiresEscalation bool
	EscalationReason   string
}

var escalationRules = []struct {
	keywords []string
	reason   string
}{
	{
		keywords: []string{"reembolso", "devolución", "devolucion", "cancelación", "cancelacion", "cancelar"},
		reason:   "Solicitud de cancelación con reembolso / autorización especial",
	},
	{
		keywords: []string{"fuerza mayor", "tormenta", "huelga", "vuelo cancelado", "reprogramar", "reprogramación", "reprogramacion"},
		reason:   "Reprogramación compleja o disrupción por fuerza mayor",
	},
	{
		keywords: []string{"agente", "humano", "operador", "reclamo", "queja", "cobro indebido", "doble cobro"},
		reason:   "Solicitud de atención especializada por agente humano",
	},
}

func ClassifyAndRespond(message string, res *ReservationInfo) ClassificationResult {
	lower := strings.ToLower(strings.TrimSpace(message))

	for _, rule := range escalationRules {
		for _, kw := range rule.keywords {
			if strings.Contains(lower, kw) {
				return ClassificationResult{
					Reply:              "Tu solicitud requiere autorización especial de tu agencia de viajes preferida. Transferimos tu caso con todo el contexto de tu reserva.",
					RequiresEscalation: true,
					EscalationReason:   rule.reason,
				}
			}
		}
	}

	if strings.Contains(lower, "equipaje") || strings.Contains(lower, "valija") || strings.Contains(lower, "maleta") {
		prov := "la aerolínea"
		if res != nil && res.Proveedor != "" {
			prov = res.Proveedor
		}
		return ClassificationResult{
			Reply: fmt.Sprintf("Tu tarifa con %s incluye un equipaje de mano de hasta 10 kg y una mochila personal. Si deseas despachar bodega adicional, puedes agregarlo con un 20%% de descuento desde el portal de %s.", prov, prov),
		}
	}

	if strings.Contains(lower, "check-in") || strings.Contains(lower, "checkin") {
		if res != nil && res.TipoReserva == "HOTEL" {
			return ClassificationResult{
				Reply: fmt.Sprintf("El check-in de tu reserva en %s (%s) está previsto a partir de %s (%s). Podés presentar tu código %s en recepción.",
					res.Proveedor, res.Estado, res.FechaInicioUTC, res.ZonaHorariaOrigen, res.CodigoConfirmacion),
			}
		}
		if res != nil {
			return ClassificationResult{
				Reply: fmt.Sprintf("El check-in online para tu reserva %s con %s (código %s) se habilita 24 horas antes de la salida (%s, zona %s).",
					res.TipoReserva, res.Proveedor, res.CodigoConfirmacion, res.FechaInicioUTC, res.ZonaHorariaOrigen),
			}
		}
		return ClassificationResult{
			Reply: "El check-in online suele habilitarse 24 horas antes de la salida de tu vuelo o a partir de las 15:00 hora local en hoteles.",
		}
	}

	if strings.Contains(lower, "estado") || strings.Contains(lower, "puerta") || strings.Contains(lower, "vuelo") || strings.Contains(lower, "horario") || strings.Contains(lower, "reserva") {
		if res == nil {
			return ClassificationResult{
				Reply: "No encontré una reserva seleccionada en este momento. Verificá tu panel de viajes activos o indicame el código de reserva.",
			}
		}
		extra := ""
		if res.Detalles != nil {
			if gate, ok := res.Detalles["puertaEmbarque"].(string); ok && gate != "" {
				extra += fmt.Sprintf(" Puerta de embarque: %s.", gate)
			}
			if flight, ok := res.Detalles["numeroVuelo"].(string); ok && flight != "" {
				extra += fmt.Sprintf(" Vuelo: %s.", flight)
			}
		}
		return ClassificationResult{
			Reply: fmt.Sprintf("Tu reserva %s con %s (código %s) figura en estado %s. Inicio programado: %s (%s).%s",
				res.TipoReserva, res.Proveedor, res.CodigoConfirmacion, res.Estado, res.FechaInicioUTC, res.ZonaHorariaOrigen, extra),
		}
	}

	return ClassificationResult{
		Reply: "Soy el Asistente Virtual de Trotamundos (Nivel 1). Puedo ayudarte con el estado de tu vuelo, equipaje o horarios de check-in; para cancelaciones con reembolso o reprogramaciones complejas derivo tu caso con ticket a tu agencia preferida.",
	}
}
