package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/models"
	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/ports/out"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (r *PGRepository) GetUser(ctx context.Context, usuarioID string) (models.UsuarioResumen, error) {
	var u models.UsuarioResumen
	err := r.pool.QueryRow(ctx, `
		SELECT usuario_id, nombre, email, telefono, idioma_pref, moneda_pref
		FROM usuario WHERE usuario_id = $1
	`, usuarioID).Scan(&u.UsuarioID, &u.Nombre, &u.Email, &u.Telefono, &u.IdiomaPref, &u.MonedaPref)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, models.ErrNotFound
	}
	return u, err
}

func (r *PGRepository) GetActiveTripsWithReservations(ctx context.Context, usuarioID string) ([]models.Viaje, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT viaje_id, usuario_id, nombre_viaje, destino_principal,
		       to_char(fecha_inicio, 'YYYY-MM-DD'), to_char(fecha_fin, 'YYYY-MM-DD'), estado
		FROM viaje
		WHERE usuario_id = $1 AND estado = 'ACTIVO'
		ORDER BY fecha_inicio ASC
	`, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trips []models.Viaje
	for rows.Next() {
		var v models.Viaje
		if err := rows.Scan(&v.ViajeID, &v.UsuarioID, &v.NombreViaje, &v.DestinoPrincipal, &v.FechaInicio, &v.FechaFin, &v.Estado); err != nil {
			return nil, err
		}
		v.Reservas = []models.Reserva{}
		trips = append(trips, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range trips {
		resRows, err := r.pool.Query(ctx, `
			SELECT reserva_id, viaje_id, tipo_reserva, proveedor, codigo_confirmacion, estado,
			       fecha_inicio_utc, fecha_fin_utc, zona_horaria_origen, zona_horaria_destino,
			       monto, moneda_iso, agregada_manualmente, detalles_jsonb
			FROM reserva
			WHERE viaje_id = $1
			ORDER BY fecha_inicio_utc ASC
		`, trips[i].ViajeID)
		if err != nil {
			return nil, err
		}
		for resRows.Next() {
			var res models.Reserva
			var detRaw []byte
			if err := resRows.Scan(
				&res.ReservaID, &res.ViajeID, &res.TipoReserva, &res.Proveedor, &res.CodigoConfirmacion,
				&res.Estado, &res.FechaInicioUTC, &res.FechaFinUTC, &res.ZonaHorariaOrigen, &res.ZonaHorariaDestino,
				&res.Monto, &res.MonedaISO, &res.AgregadaManualmente, &detRaw,
			); err != nil {
				resRows.Close()
				return nil, err
			}
			_ = json.Unmarshal(detRaw, &res.Detalles)
			trips[i].Reservas = append(trips[i].Reservas, res)
		}
		resRows.Close()
		trips[i].TotalReservas = len(trips[i].Reservas)
	}
	return trips, nil
}

func (r *PGRepository) GetReservationContext(ctx context.Context, reservaID string) (models.ReservaContexto, error) {
	var outCtx models.ReservaContexto
	var detRaw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT u.usuario_id, u.nombre, u.email, u.telefono, u.idioma_pref, u.moneda_pref,
		       v.viaje_id, v.usuario_id, v.nombre_viaje, v.destino_principal,
		       to_char(v.fecha_inicio, 'YYYY-MM-DD'), to_char(v.fecha_fin, 'YYYY-MM-DD'), v.estado,
		       r.reserva_id, r.viaje_id, r.tipo_reserva, r.proveedor, r.codigo_confirmacion, r.estado,
		       r.fecha_inicio_utc, r.fecha_fin_utc, r.zona_horaria_origen, r.zona_horaria_destino,
		       r.monto, r.moneda_iso, r.agregada_manualmente, r.detalles_jsonb
		FROM reserva r
		JOIN viaje v ON v.viaje_id = r.viaje_id
		JOIN usuario u ON u.usuario_id = v.usuario_id
		WHERE r.reserva_id = $1
	`, reservaID).Scan(
		&outCtx.Usuario.UsuarioID, &outCtx.Usuario.Nombre, &outCtx.Usuario.Email, &outCtx.Usuario.Telefono,
		&outCtx.Usuario.IdiomaPref, &outCtx.Usuario.MonedaPref,
		&outCtx.Viaje.ViajeID, &outCtx.Viaje.UsuarioID, &outCtx.Viaje.NombreViaje, &outCtx.Viaje.DestinoPrincipal,
		&outCtx.Viaje.FechaInicio, &outCtx.Viaje.FechaFin, &outCtx.Viaje.Estado,
		&outCtx.Reserva.ReservaID, &outCtx.Reserva.ViajeID, &outCtx.Reserva.TipoReserva, &outCtx.Reserva.Proveedor,
		&outCtx.Reserva.CodigoConfirmacion, &outCtx.Reserva.Estado,
		&outCtx.Reserva.FechaInicioUTC, &outCtx.Reserva.FechaFinUTC, &outCtx.Reserva.ZonaHorariaOrigen,
		&outCtx.Reserva.ZonaHorariaDestino, &outCtx.Reserva.Monto, &outCtx.Reserva.MonedaISO,
		&outCtx.Reserva.AgregadaManualmente, &detRaw,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return outCtx, models.ErrNotFound
	}
	if err != nil {
		return outCtx, err
	}
	_ = json.Unmarshal(detRaw, &outCtx.Reserva.Detalles)
	return outCtx, nil
}

func (r *PGRepository) GetTripByID(ctx context.Context, viajeID string) (models.Viaje, error) {
	var v models.Viaje
	err := r.pool.QueryRow(ctx, `
		SELECT viaje_id, usuario_id, nombre_viaje, destino_principal,
		       to_char(fecha_inicio, 'YYYY-MM-DD'), to_char(fecha_fin, 'YYYY-MM-DD'), estado
		FROM viaje WHERE viaje_id = $1
	`, viajeID).Scan(&v.ViajeID, &v.UsuarioID, &v.NombreViaje, &v.DestinoPrincipal, &v.FechaInicio, &v.FechaFin, &v.Estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, models.ErrNotFound
	}
	return v, err
}

func (r *PGRepository) SaveTrip(ctx context.Context, trip models.Viaje) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO viaje (viaje_id, usuario_id, nombre_viaje, destino_principal, fecha_inicio, fecha_fin, estado)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7)
	`, trip.ViajeID, trip.UsuarioID, trip.NombreViaje, trip.DestinoPrincipal, trip.FechaInicio, trip.FechaFin, trip.Estado)
	return err
}

func (r *PGRepository) SaveReservation(ctx context.Context, res models.Reserva, idempotencyKey string) error {
	detRaw, _ := json.Marshal(res.Detalles)
	var idem *string
	if idempotencyKey != "" {
		idem = &idempotencyKey
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO reserva (
			reserva_id, viaje_id, tipo_reserva, proveedor, codigo_confirmacion, idempotency_key,
			estado, fecha_inicio_utc, fecha_fin_utc, zona_horaria_origen, zona_horaria_destino,
			monto, moneda_iso, agregada_manualmente, detalles_jsonb
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, res.ReservaID, res.ViajeID, res.TipoReserva, res.Proveedor, res.CodigoConfirmacion, idem,
		res.Estado, res.FechaInicioUTC, res.FechaFinUTC, res.ZonaHorariaOrigen, res.ZonaHorariaDestino,
		res.Monto, res.MonedaISO, res.AgregadaManualmente, detRaw)
	return err
}

func (r *PGRepository) ArchiveTrip(ctx context.Context, viajeID string, archivedAt time.Time) (models.Viaje, error) {
	var v models.Viaje
	err := r.pool.QueryRow(ctx, `
		UPDATE viaje SET estado = 'ARCHIVADO', archivado_en = $2
		WHERE viaje_id = $1
		RETURNING viaje_id, usuario_id, nombre_viaje, destino_principal,
		          to_char(fecha_inicio, 'YYYY-MM-DD'), to_char(fecha_fin, 'YYYY-MM-DD'), estado
	`, viajeID, archivedAt).Scan(&v.ViajeID, &v.UsuarioID, &v.NombreViaje, &v.DestinoPrincipal, &v.FechaInicio, &v.FechaFin, &v.Estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, models.ErrNotFound
	}
	return v, err
}

func (r *PGRepository) GetIdempotency(ctx context.Context, key string) (*out.IdempotencyRecord, error) {
	var rec out.IdempotencyRecord
	err := r.pool.QueryRow(ctx, `
		SELECT request_hash, status_code, response_body
		FROM idempotency_keys WHERE idempotency_key = $1
	`, key).Scan(&rec.RequestHash, &rec.StatusCode, &rec.ResponseBody)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *PGRepository) SaveIdempotency(ctx context.Context, key string, rec out.IdempotencyRecord) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO idempotency_keys (idempotency_key, request_hash, status_code, response_body)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, key, rec.RequestHash, rec.StatusCode, rec.ResponseBody)
	return err
}
