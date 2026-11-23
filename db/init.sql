-- Esquema OLTP del Prototipo Acotado "Trotamundos" (alineado con 05-Vistas-Arquitectonicas §5)

CREATE TABLE IF NOT EXISTS usuario (
    usuario_id       TEXT PRIMARY KEY,
    email            TEXT UNIQUE NOT NULL,
    nombre           TEXT NOT NULL,
    telefono         TEXT NOT NULL DEFAULT '',
    idioma_pref      TEXT NOT NULL DEFAULT 'es-AR',
    moneda_pref      TEXT NOT NULL DEFAULT 'USD',
    creado_en        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS viaje (
    viaje_id          TEXT PRIMARY KEY,
    usuario_id        TEXT NOT NULL REFERENCES usuario(usuario_id) ON DELETE CASCADE,
    nombre_viaje      TEXT NOT NULL,
    fecha_inicio      DATE NOT NULL,
    fecha_fin         DATE NOT NULL,
    destino_principal TEXT NOT NULL,
    estado            TEXT NOT NULL DEFAULT 'ACTIVO' CHECK (estado IN ('ACTIVO', 'COMPLETADO', 'ARCHIVADO')),
    archivado_en      TIMESTAMPTZ,
    creado_en         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_viaje_usuario_estado ON viaje (usuario_id, estado);

CREATE TABLE IF NOT EXISTS reserva (
    reserva_id           TEXT PRIMARY KEY,
    viaje_id             TEXT NOT NULL REFERENCES viaje(viaje_id) ON DELETE CASCADE,
    tipo_reserva         TEXT NOT NULL CHECK (tipo_reserva IN ('VUELO', 'HOTEL', 'AUTO', 'TREN', 'BUS')),
    proveedor            TEXT NOT NULL,
    codigo_confirmacion  TEXT NOT NULL,
    idempotency_key      TEXT UNIQUE,
    estado               TEXT NOT NULL DEFAULT 'CONFIRMADA' CHECK (estado IN ('CONFIRMADA', 'CANCELADA', 'REPROGRAMADA')),
    fecha_inicio_utc     TIMESTAMPTZ NOT NULL,
    fecha_fin_utc        TIMESTAMPTZ NOT NULL,
    zona_horaria_origen  TEXT NOT NULL DEFAULT 'UTC',
    zona_horaria_destino TEXT NOT NULL DEFAULT 'UTC',
    monto                NUMERIC(12, 2) NOT NULL DEFAULT 0,
    moneda_iso           TEXT NOT NULL DEFAULT 'USD',
    agregada_manualmente BOOLEAN NOT NULL DEFAULT TRUE,
    detalles_jsonb       JSONB NOT NULL DEFAULT '{}'::jsonb,
    creado_en            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reserva_viaje ON reserva (viaje_id);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    idempotency_key TEXT PRIMARY KEY,
    request_hash    TEXT NOT NULL,
    status_code     INT NOT NULL,
    response_body   JSONB NOT NULL,
    creado_en       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sesion_chat (
    sesion_id     TEXT PRIMARY KEY,
    usuario_id    TEXT NOT NULL REFERENCES usuario(usuario_id) ON DELETE CASCADE,
    estado        TEXT NOT NULL DEFAULT 'ABIERTA' CHECK (estado IN ('ABIERTA', 'CERRADA', 'DERIVADA')),
    inicio_sesion TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mensaje_chat (
    mensaje_id TEXT PRIMARY KEY,
    sesion_id  TEXT NOT NULL REFERENCES sesion_chat(sesion_id) ON DELETE CASCADE,
    emisor     TEXT NOT NULL CHECK (emisor IN ('USUARIO', 'BOT')),
    contenido  TEXT NOT NULL,
    enviado_en TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ticket_derivacion (
    ticket_id             TEXT PRIMARY KEY,
    sesion_id             TEXT NOT NULL REFERENCES sesion_chat(sesion_id) ON DELETE CASCADE,
    usuario_id            TEXT NOT NULL REFERENCES usuario(usuario_id) ON DELETE CASCADE,
    reserva_id            TEXT REFERENCES reserva(reserva_id) ON DELETE SET NULL,
    agencia_destino       TEXT NOT NULL,
    motivo_derivacion     TEXT NOT NULL,
    payload_json          JSONB NOT NULL,
    estado                TEXT NOT NULL DEFAULT 'PENDIENTE' CHECK (estado IN ('PENDIENTE', 'ENTREGADO', 'RESUELTO')),
    idempotency_key       TEXT UNIQUE NOT NULL,
    agente_asignado       TEXT NOT NULL DEFAULT '',
    intentos              INT NOT NULL DEFAULT 0,
    ultimo_error          TEXT NOT NULL DEFAULT '',
    proximo_reintento_utc TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    creado_en             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    entregado_en          TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ticket_outbox_pendiente ON ticket_derivacion (estado, proximo_reintento_utc);
CREATE INDEX IF NOT EXISTS idx_ticket_usuario ON ticket_derivacion (usuario_id, creado_en DESC);

-- Datos semilla para demostración en vivo (alineados con 06-Especificaciones-API)
INSERT INTO usuario (usuario_id, email, nombre, telefono, idioma_pref, moneda_pref)
VALUES
    ('usr_9812a3', 'nicola@example.com', 'Nicola Satragni', '+54 9 2494 123456', 'es-AR', 'USD')
ON CONFLICT (usuario_id) DO NOTHING;

INSERT INTO viaje (viaje_id, usuario_id, nombre_viaje, fecha_inicio, fecha_fin, destino_principal, estado)
VALUES
    ('vj_5501', 'usr_9812a3', 'Gira Europea 2026', '2026-11-10', '2026-11-25', 'Madrid, España', 'ACTIVO')
ON CONFLICT (viaje_id) DO NOTHING;

INSERT INTO reserva (
    reserva_id, viaje_id, tipo_reserva, proveedor, codigo_confirmacion, idempotency_key,
    estado, fecha_inicio_utc, fecha_fin_utc, zona_horaria_origen, zona_horaria_destino,
    monto, moneda_iso, agregada_manualmente, detalles_jsonb
)
VALUES
    (
        'res_8819', 'vj_5501', 'VUELO', 'Iberia', 'IB-7412A', 'seed-res-8819',
        'CONFIRMADA', '2026-11-10T08:30:00Z', '2026-11-10T20:15:00Z',
        'America/Argentina/Buenos_Aires', 'Europe/Madrid',
        945.50, 'USD', FALSE,
        '{"origen":"EZE","destino":"MAD","numeroVuelo":"IB6844","asiento":"14A","terminal":"T4","puertaEmbarque":"B12"}'::jsonb
    ),
    (
        'res_8820', 'vj_5501', 'HOTEL', 'NH Collection Madrid', 'NH-90321', 'seed-res-8820',
        'CONFIRMADA', '2026-11-10T22:00:00Z', '2026-11-15T10:00:00Z',
        'Europe/Madrid', 'Europe/Madrid',
        620.00, 'EUR', FALSE,
        '{"nombreHotel":"NH Collection Madrid Eurobuilding","direccion":"C. de Padre Damián 23, Madrid","tipoHabitacion":"Superior Doble","horaCheckinUtc":"2026-11-10T15:00:00Z"}'::jsonb
    )
ON CONFLICT (reserva_id) DO NOTHING;
