package models

import (
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("resource not found")
	ErrValidation          = errors.New("validation error")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different payload")
)

type UsuarioResumen struct {
	UsuarioID  string `json:"usuarioId"`
	Nombre     string `json:"nombre"`
	Email      string `json:"email"`
	Telefono   string `json:"telefono"`
	IdiomaPref string `json:"idiomaPref"`
	MonedaPref string `json:"monedaPref"`
}

type Reserva struct {
	ReservaID           string         `json:"reservaId"`
	ViajeID             string         `json:"viajeId"`
	TipoReserva         string         `json:"tipoReserva"`
	Proveedor           string         `json:"proveedor"`
	CodigoConfirmacion  string         `json:"codigoConfirmacion"`
	Estado              string         `json:"estado"`
	FechaInicioUTC      time.Time      `json:"fechaInicioUtc"`
	FechaFinUTC         time.Time      `json:"fechaFinUtc"`
	ZonaHorariaOrigen   string         `json:"zonaHorariaOrigen"`
	ZonaHorariaDestino  string         `json:"zonaHorariaDestino"`
	Monto               float64        `json:"monto"`
	MonedaISO           string         `json:"monedaIso"`
	AgregadaManualmente bool           `json:"agregadaManualmente"`
	Detalles            map[string]any `json:"detalles,omitempty"`
}

type Viaje struct {
	ViajeID          string    `json:"viajeId"`
	UsuarioID        string    `json:"usuarioId"`
	NombreViaje      string    `json:"nombreViaje"`
	DestinoPrincipal string    `json:"destinoPrincipal"`
	FechaInicio      string    `json:"fechaInicio"`
	FechaFin         string    `json:"fechaFin"`
	Estado           string    `json:"estado"`
	TotalReservas    int       `json:"totalReservas"`
	Reservas         []Reserva `json:"reservas"`
}

type ItinerarioConsolidado struct {
	UsuarioID       string    `json:"usuarioId"`
	Timestamp       time.Time `json:"timestamp"`
	Idioma          string    `json:"idioma"`
	MonedaPreferida string    `json:"monedaPreferida"`
	OrigenDatos     string    `json:"origenDatos"`
	ViajesActivos   []Viaje   `json:"viajesActivos"`
}

type ReservaContexto struct {
	Usuario UsuarioResumen `json:"usuario"`
	Viaje   Viaje          `json:"viaje"`
	Reserva Reserva        `json:"reserva"`
}

type CreateTripInput struct {
	UsuarioID        string `json:"usuarioId"`
	NombreViaje      string `json:"nombreViaje"`
	DestinoPrincipal string `json:"destinoPrincipal"`
	FechaInicio      string `json:"fechaInicio"`
	FechaFin         string `json:"fechaFin"`
}

type CreateReservationInput struct {
	ViajeID            string         `json:"viajeId"`
	TipoReserva        string         `json:"tipoReserva"`
	Proveedor          string         `json:"proveedor"`
	CodigoConfirmacion string         `json:"codigoConfirmacion"`
	FechaInicioUTC     time.Time      `json:"fechaInicioUtc"`
	FechaFinUTC        time.Time      `json:"fechaFinUtc"`
	ZonaHorariaOrigen  string         `json:"zonaHorariaOrigen"`
	ZonaHorariaDestino string         `json:"zonaHorariaDestino"`
	Monto              float64        `json:"monto"`
	MonedaISO          string         `json:"monedaIso"`
	Detalles           map[string]any `json:"detalles"`
}

type ItineraryUpdatedEvent struct {
	EventID   string    `json:"eventId"`
	UsuarioID string    `json:"usuarioId"`
	ViajeID   string    `json:"viajeId"`
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}
