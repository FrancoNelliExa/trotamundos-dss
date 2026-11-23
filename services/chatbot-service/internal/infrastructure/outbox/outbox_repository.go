package outbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TicketRecord struct {
	TicketID            string     `json:"ticketId"`
	SesionID            string     `json:"sesionId"`
	UsuarioID           string     `json:"usuarioId"`
	ReservaID           string     `json:"reservaId,omitempty"`
	AgenciaDestino      string     `json:"agenciaDestino"`
	MotivoDerivacion    string     `json:"motivoDerivacion"`
	PayloadJSON         []byte     `json:"-"`
	Estado              string     `json:"estado"`
	IdempotencyKey      string     `json:"idempotencyKey"`
	AgenteAsignado      string     `json:"agenteAsignado,omitempty"`
	Intentos            int        `json:"intentos"`
	UltimoError         string     `json:"ultimoError,omitempty"`
	ProximoReintentoUTC time.Time  `json:"proximoReintentoUtc"`
	CreadoEn            time.Time  `json:"creadoEn"`
	EntregadoEn         *time.Time `json:"entregadoEn,omitempty"`
}

type OutboxStore interface {
	FetchPendingDue(ctx context.Context, limit int) ([]TicketRecord, error)
	MarkDelivered(ctx context.Context, ticketID, agente string) error
	MarkRetry(ctx context.Context, ticketID string, attempts int, nextRetry time.Time, lastErr string) error
}

type PGOutboxRepository struct {
	pool *pgxpool.Pool
}

func NewPGOutboxRepository(pool *pgxpool.Pool) *PGOutboxRepository {
	return &PGOutboxRepository{pool: pool}
}

func (r *PGOutboxRepository) EnsureSession(ctx context.Context, sesionID, usuarioID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sesion_chat (sesion_id, usuario_id, estado)
		VALUES ($1, $2, 'ABIERTA')
		ON CONFLICT (sesion_id) DO NOTHING
	`, sesionID, usuarioID)
	return err
}

func (r *PGOutboxRepository) SaveMessage(ctx context.Context, mensajeID, sesionID, emisor, contenido string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO mensaje_chat (mensaje_id, sesion_id, emisor, contenido)
		VALUES ($1, $2, $3, $4)
	`, mensajeID, sesionID, emisor, contenido)
	return err
}

func (r *PGOutboxRepository) MarkSessionEscalated(ctx context.Context, sesionID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sesion_chat SET estado = 'DERIVADA' WHERE sesion_id = $1`, sesionID)
	return err
}

func (r *PGOutboxRepository) SaveTicket(ctx context.Context, tk TicketRecord) error {
	var resID *string
	if tk.ReservaID != "" {
		resID = &tk.ReservaID
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ticket_derivacion (
			ticket_id, sesion_id, usuario_id, reserva_id, agencia_destino, motivo_derivacion,
			payload_json, estado, idempotency_key, agente_asignado, intentos, ultimo_error,
			proximo_reintento_utc, entregado_en
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, tk.TicketID, tk.SesionID, tk.UsuarioID, resID, tk.AgenciaDestino, tk.MotivoDerivacion,
		tk.PayloadJSON, tk.Estado, tk.IdempotencyKey, tk.AgenteAsignado, tk.Intentos, tk.UltimoError,
		tk.ProximoReintentoUTC, tk.EntregadoEn)
	return err
}

func (r *PGOutboxRepository) ListTicketsByUser(ctx context.Context, usuarioID string) ([]TicketRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ticket_id, sesion_id, usuario_id, COALESCE(reserva_id, ''), agencia_destino, motivo_derivacion,
		       estado, idempotency_key, agente_asignado, intentos, ultimo_error,
		       proximo_reintento_utc, creado_en, entregado_en
		FROM ticket_derivacion
		WHERE usuario_id = $1
		ORDER BY creado_en DESC
		LIMIT 50
	`, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []TicketRecord{}
	for rows.Next() {
		var tk TicketRecord
		if err := rows.Scan(
			&tk.TicketID, &tk.SesionID, &tk.UsuarioID, &tk.ReservaID, &tk.AgenciaDestino, &tk.MotivoDerivacion,
			&tk.Estado, &tk.IdempotencyKey, &tk.AgenteAsignado, &tk.Intentos, &tk.UltimoError,
			&tk.ProximoReintentoUTC, &tk.CreadoEn, &tk.EntregadoEn,
		); err != nil {
			return nil, err
		}
		list = append(list, tk)
	}
	return list, rows.Err()
}

func (r *PGOutboxRepository) FetchPendingDue(ctx context.Context, limit int) ([]TicketRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ticket_id, sesion_id, usuario_id, COALESCE(reserva_id, ''), agencia_destino, motivo_derivacion,
		       payload_json, estado, idempotency_key, intentos
		FROM ticket_derivacion
		WHERE estado = 'PENDIENTE' AND proximo_reintento_utc <= NOW()
		ORDER BY creado_en ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []TicketRecord
	for rows.Next() {
		var tk TicketRecord
		if err := rows.Scan(
			&tk.TicketID, &tk.SesionID, &tk.UsuarioID, &tk.ReservaID, &tk.AgenciaDestino,
			&tk.MotivoDerivacion, &tk.PayloadJSON, &tk.Estado, &tk.IdempotencyKey, &tk.Intentos,
		); err != nil {
			return nil, err
		}
		list = append(list, tk)
	}
	return list, rows.Err()
}

func (r *PGOutboxRepository) MarkDelivered(ctx context.Context, ticketID, agente string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE ticket_derivacion
		SET estado = 'ENTREGADO', agente_asignado = $2, ultimo_error = '', entregado_en = NOW()
		WHERE ticket_id = $1
	`, ticketID, agente)
	return err
}

func (r *PGOutboxRepository) MarkRetry(ctx context.Context, ticketID string, attempts int, nextRetry time.Time, lastErr string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE ticket_derivacion
		SET intentos = $2, proximo_reintento_utc = $3, ultimo_error = $4
		WHERE ticket_id = $1
	`, ticketID, attempts, nextRetry, lastErr)
	return err
}
