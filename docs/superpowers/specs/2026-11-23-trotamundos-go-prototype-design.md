# Diseño del Prototipo Acotado — Chatbot (Opción B) y Núcleo de Itinerarios en Go

**Fecha:** 2026-11-23  
**Repositorio:** `trotamundos-dss`  
**Equipo:** Nicola Satragni, Franco Nelli  
**Base arquitectónica:** Wiki de arquitectura (`01` a `06`), ADD 3.0, ADRs MADR (`ADR-001`, `ADR-002`, `ADR-004`, `ADR-005`, `ADR-006`, `ADR-010`, `ADR-013`, `ADR-014`) y contratos OpenAPI 3.0.

---

## 1. Propósito y Alcance Acotado

Construir un **prototipo acotado, ejecutable y sin sobreingeniería** en **Go** centrado en demostrar el módulo avanzado obligatorio de la 2da entrega (**Opción B — Asistente Virtual / Chatbot**) integrado con el núcleo de itinerarios, respetando fielmente los contratos de [06‐Especificaciones‐API.md](trotamundos-dss.wiki/06‐Especificaciones‐API.md) y la estructura hexagonal de [05‐Vistas‐Arquitectonicas.md](trotamundos-dss.wiki/05‐Vistas‐Arquitectonicas.md).

Conforme al punto 2 del pliego (*"es fundamental que los grupos acoten el alcance del prototipo"*), el prototipo implementa **4 servicios Go pequeños** sobre **Docker Compose (PostgreSQL + Redis + Redpanda/Kafka)** y un **frontend mínimo HTML/CSS/JS** sin frameworks ni dependencias pesadas.

---

## 2. Qué entra y qué queda fuera (Scope Acotado)

### Incluido en el prototipo
1. **`chatbot-service` (Foco principal — Opción B, `ADR-013` y `ADR-014`):**
   * `POST /api/v1/chatbot/consulta`: persiste `sesion_chat` y `mensaje_chat` en PostgreSQL, consulta el contexto de la reserva en `itinerary-service`, clasifica la intención (Nivel 1 autónomo vs. derivación compleja) y deriva a la Agencia Preferida con ticket + `Idempotency-Key`.
   * **Circuit Breaker** (`timeout = 2.5 s`, apertura tras `5` fallos consecutivos en `30 s`, `Half-Open` tras `30 s`, *fail-fast* `< 20 ms`).
   * **Cola Outbox en PostgreSQL (`ticket_derivacion`):** ante timeout, error `5xx` o circuito abierto guarda el ticket como `PENDIENTE`, y un *worker* en segundo plano reintenta con la misma `Idempotency-Key` hasta que la agencia responde `200 OK` (`ENTREGADO`).
   * Publica eventos de derivación en Kafka (`chatbot.events`) y expone `GET /api/v1/chatbot/tickets/{usuarioId}` y `GET /api/v1/chatbot/circuit-status` para inspección en vivo.
2. **`itinerary-service` (Núcleo mínimo en Arquitectura Hexagonal — `ADR-005`, `ADR-006`, `ADR-010`):**
   * Estructura estricta `domain/models`, `domain/ports/in`, `domain/ports/out`, `application/services`, `infrastructure/adapters/{in,out}`.
   * `GET /api/v1/itinerarios/{usuarioId}` con **Redis Cache-Aside** (`CACHE_HIT` / `CACHE_MISS`, TTL `15 min`).
   * `GET /api/v1/reservas/{reservaId}` (contexto para el Chatbot).
   * `POST /api/v1/viajes`, `POST /api/v1/reservas` (con `Idempotency-Key`, fechas UTC + IANA TZ + moneda ISO 4217), `PUT /api/v1/viajes/{viajeId}/archivar`.
   * Invalida caché en Redis y publica `ItineraryUpdatedEvent` en Kafka (`itinerary.updated`) en cada escritura.
3. **`web-bff` (`:8080`, `ADR-004`):**
   * Punto de entrada público único: enruta `/api/v1/itinerarios/*`, `/api/v1/viajes/*`, `/api/v1/reservas/*`, `/api/v1/chatbot/*` y el control demo de la agencia; valida JWT Bearer (con endpoint demo `POST /api/v1/auth/demo-token` sin necesidad de un `auth-service` aparte); sirve `api/openapi.yaml` (`/openapi.yaml`), `/docs` y el frontend mínimo `web/` en `/`.
4. **`agency-mock` (Simulador liviano de Agencia Preferida — ~80 líneas Go):**
   * Implementa `POST /api/v1/agencia/derivacion` (contrato exacto de `06` §4.2, idempotente por `Idempotency-Key`).
   * Expone `POST /mock/mode` (`healthy` | `slow` [latencia `5 s` > `2.5 s`] | `error500`) para probar Circuit Breaker + Outbox en vivo.
5. **Frontend mínimo (`web/index.html`, `web/styles.css`, `web/app.js`):**
   * Panel simple de viajes/reservas (con badge `CACHE_HIT`/`CACHE_MISS`, latencia medida y alta rápida de reserva manual).
   * Chat interactivo del Asistente Virtual (botones de prueba rápida Nivel 1 vs. Derivación Compleja), indicador en vivo del **Circuit Breaker**, tabla de tickets **Outbox** (`PENDIENTE` → `ENTREGADO`) y selector de estado de la **Agencia Externa** (`Saludable` / `Lenta 5s` / `Error 500`).
6. **Sincronización Wiki (`trotamundos-dss.wiki/`):**
   * Actualizar `05‐Vistas‐Arquitectonicas.md` §2 Vista de Módulos de `.ts` a `.go` reflejando el código real.
   * Actualizar `04‐ADRs.md` (`ADR-013` → `Aceptado`) y `03‐Iteraciones‐ADD.md` (cierre de Kanban indicando el alcance validado del prototipo).

### Fuera de alcance en este prototipo (documentados en la arquitectura pero no implementados para evitar sobreingeniería)
* `email-scanner-worker`, `analytics-consumer`, `auth-service` independiente, `mobile-bff`, compartición HMAC y adaptadores de múltiples aerolíneas/hoteles/trenes.

---

## 3. Enfoque Elegido y Stack

* **Enfoque elegido:** Enfoque 3 acotado — **Docker Compose** con **PostgreSQL 16 + Redis 7 + Redpanda (Kafka compatible)** reales + **4 servicios Go** usando `net/http` de la librería estándar.
* **Dependencias Go mínimas (3 librerías + stdlib):**
  * `github.com/jackc/pgx/v5` (PostgreSQL)
  * `github.com/redis/go-redis/v9` (Redis)
  * `github.com/segmentio/kafka-go` (Kafka / Redpanda)
  * `github.com/golang-jwt/jwt/v5` (JWT `RS256` en `web-bff`)
* **Puertos en el host:** `web-bff: 8080`, `postgres: 5433`, `redis: 6380`, `redpanda: 19092`. Los servicios internos (`itinerary-service:8081`, `chatbot-service:8082`, `agency-mock:8089`) corren solo en la red interna de Docker.

---

## 4. Estructura del Repositorio

```text
trotamundos-dss/
├── api/
│   └── openapi.yaml                          # Contrato OpenAPI 3.0.3 (Spec-driven)
├── db/
│   └── init.sql                              # Tablas OLTP mínimas + datos semilla demo
├── docker-compose.yml                        # Postgres, Redis, Redpanda + 4 servicios Go
├── Dockerfile                                # Build multi-stage por SERVICE
├── go.mod / go.sum
├── pkg/
│   ├── httputil/problem.go                   # Errores RFC 7807, JSON helpers, CORS
│   └── resilience/circuit_breaker.go         # Circuit Breaker (2.5s timeout, 5 fallos/30s, 30s Half-Open)
├── services/
│   ├── itinerary-service/                    # Arquitectura Hexagonal (ADR-010)
│   │   ├── cmd/main.go
│   │   └── internal/
│   │       ├── domain/
│   │       │   ├── models/                   # trip.go, reservation.go
│   │       │   └── ports/
│   │       │       ├── in/                   # itinerary_use_cases.go
│   │       │       └── out/                  # repository_port.go, cache_port.go, event_publisher_port.go
│   │       ├── application/services/         # itinerary_service.go
│   │       └── infrastructure/adapters/
│   │           ├── in/                       # rest_itinerary_controller.go
│   │           └── out/
│   │               ├── postgres/             # pg_repository.go
│   │               ├── redis/                # redis_cache.go
│   │               └── kafka/                # kafka_publisher.go
│   ├── chatbot-service/                      # Módulo Avanzado Opción B (ADR-013, ADR-014)
│   │   ├── cmd/main.go
│   │   └── internal/
│   │       ├── domain/                       # intent_classifier.go
│   │       └── infrastructure/
│   │           ├── adapters/                 # rest_chatbot_controller.go, itinerary_client.go, agency_webhook_client.go
│   │           └── outbox/                   # outbox_repository.go, outbox_worker.go
│   ├── web-bff/                              # BFF Web + JWT + UI estática + /openapi.yaml (ADR-004)
│   │   ├── cmd/main.go
│   │   └── internal/                         # router.go, auth.go
│   └── agency-mock/                          # Mock de Agencia Preferida con inyección de fallas
│       └── cmd/main.go
├── web/                                      # Frontend mínimo (HTML + CSS + JS vanilla)
│   ├── index.html
│   ├── styles.css
│   └── app.js
└── scripts/
    └── demo_e2e.sh                           # Prueba automatizada contra Docker Compose
```

---

## 5. Verificación

1. **Tests unitarios (`go test ./...`):**
   * `pkg/resilience`: máquina de estados del Circuit Breaker (`Closed` → 5 fallos → `Open` + *fail-fast* `< 20 ms` → `Half-Open` → `Closed`, timeout `2.5 s`).
   * `services/chatbot-service/internal/domain`: clasificación Nivel 1 vs. derivación compleja.
   * `services/itinerary-service/internal/application/services`: Cache-Aside (`CACHE_HIT`/`CACHE_MISS`), invalidación y degradación elegante cuando falla caché usando mocks de los puertos hexagonales.
2. **Demo / Integración E2E (`docker compose up -d --build && ./scripts/demo_e2e.sh`):**
   * Consulta de itinerario (`CACHE_MISS` → `CACHE_HIT` `< 800 ms`).
   * Consulta Nivel 1 al Chatbot (`requiereDerivacion: false`, `< 300 ms`).
   * Derivación compleja con agencia saludable (`estado: ENTREGADO`).
   * Cambio de agencia a modo `error500` / `slow` → apertura del Circuit Breaker tras 5 fallos + ticket guardado en Outbox (`estado: PENDIENTE`) → agencia vuelve a `healthy` → worker Outbox entrega automáticamente (`estado: ENTREGADO`, sin duplicados por `Idempotency-Key`).
