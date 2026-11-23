# Trotamundos Chatbot Prototype (Opción B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a lean, runnable Go prototype centered on the Opción B Chatbot (`chatbot-service` with Circuit Breaker + Outbox) integrated with a hexagonal `itinerary-service` (PostgreSQL + Redis Cache-Aside + Redpanda/Kafka), `web-bff` + minimal HTML/CSS/JS frontend, and `agency-mock`, then sync the wiki to match the Go code.

**Architecture:** Four small Go binaries (`itinerary-service`, `chatbot-service`, `web-bff`, `agency-mock`) in one root `go.mod`, using stdlib `net/http` and four minimal dependencies (`pgx/v5`, `go-redis/v9`, `kafka-go`, `jwt/v5`), orchestrated via Docker Compose alongside PostgreSQL 16, Redis 7, and Redpanda (Kafka-compatible). `web-bff` is the single public HTTP entry point (`:8080`) serving REST routes, `/openapi.yaml`, `/docs`, and the static HTML/CSS/JS UI.

**Tech Stack:** Go 1.25+ (`net/http`, `log/slog`), PostgreSQL 16, Redis 7, Redpanda (Kafka), Docker Compose, HTML/CSS/Vanilla JS.

**Spec:** [docs/superpowers/specs/2026-11-23-trotamundos-go-prototype-design.md](../specs/2026-11-23-trotamundos-go-prototype-design.md)

## Global Constraints
- Go module path: `github.com/FrancoNelliExa/trotamundos-dss`, using stdlib `net/http` only (no web frameworks).
- Allowed external Go dependencies: `github.com/jackc/pgx/v5`, `github.com/redis/go-redis/v9`, `github.com/segmentio/kafka-go`, `github.com/golang-jwt/jwt/v5`.
- All HTTP error responses follow RFC 7807 (`tipo`, `codigo`, `titulo`, `status`, `detalle`, `instancia`, `requestId`) per `06‐Especificaciones‐API.md`.
- All dates use ISO 8601 UTC (`Z`), timezones use IANA identifiers, currencies use ISO 4217, and `Accept-Language` uses BCP-47.
- Circuit Breaker constants (`ADR-014`): HTTP timeout `2.5 s`, opens after `5` consecutive failures within `30 s`, transitions to `Half-Open` after `30 s`, fails fast in `< 20 ms` while `Open`.
- Host ports: `web-bff: 8080`, `postgres: 5433`, `redis: 6380`, `redpanda: 19092`. Internal services (`itinerary-service:8081`, `chatbot-service:8082`, `agency-mock:8089`) are NOT published on the host.
- Frontend uses plain HTML, CSS, and vanilla JS only (zero Node/bundler dependencies).

## Review Focus
- **Redis unreachable during itinerary read (`itinerary-service`):** `GET /api/v1/itinerarios/{usuarioId}` must degrade gracefully to PostgreSQL (`origenDatos: "CACHE_MISS"`) instead of failing with 500 — pinned by unit test in Task 2.
- **Duplicate `Idempotency-Key` with different payload vs. same payload (`itinerary-service` & `agency-mock`):** reusing a key with identical payload returns the original result without duplicating rows; reusing with a different payload returns `409 Conflict` (RFC 7807) — pinned by tests in Tasks 2 and 3.
- **Chatbot escalation with empty or missing `reservaId`:** must still fetch the traveler's next active reservation (or degrade gracefully with a clear `400`/`404` RFC 7807 error if user has no reservations) instead of panicking — pinned by unit test in Task 4.
- **Outbox retry deduplication after agency recovery:** when `agency-mock` recovers from `slow`/`error500`, the Outbox worker must deliver every `PENDIENTE` ticket with its original `Idempotency-Key` and transition it to `ENTREGADO` without creating duplicate agency tickets — pinned by tests in Tasks 3, 4, and 6.
- **Unauthorized / cross-user access (`web-bff`):** missing/invalid JWT returns `401 Unauthorized` and a valid JWT requesting another `usuarioId` returns `403 Forbidden` in RFC 7807 format — pinned by unit test in Task 5.

---

### Task 1: Project Foundation, Shared Packages (`httputil`, `resilience`), DB Schema, and OpenAPI Contract

**Files:**
- Create: `go.mod`, `go.sum`
- Create: `pkg/httputil/problem.go`
- Create: `pkg/resilience/circuit_breaker.go`
- Test: `pkg/resilience/circuit_breaker_test.go`
- Create: `db/init.sql`
- Create: `api/openapi.yaml`

**Interfaces:**
- Produces:
  - `pkg/httputil`: `WriteJSON(w http.ResponseWriter, status int, v any)`, `WriteProblem(w http.ResponseWriter, status int, code, title, detail, instance string)`, `WithCORS(next http.Handler) http.Handler`, `RequestID(r *http.Request) string`
  - `pkg/resilience`: `type State string` (`StateClosed`, `StateOpen`, `StateHalfOpen`), `type Config struct { FailureThreshold int; FailureWindow time.Duration; ResetTimeout time.Duration }`, `type CircuitBreaker struct`, `NewCircuitBreaker(cfg Config) *CircuitBreaker`, `(*CircuitBreaker).Execute(fn func() error) error`, `(*CircuitBreaker).State() State`, `(*CircuitBreaker).Snapshot() Status`, `ErrCircuitOpen`

- [ ] **Step 1: Initialize Go module and dependencies**
  Run `go mod init github.com/FrancoNelliExa/trotamundos-dss` and add the 4 pinned dependencies (`pgx/v5`, `go-redis/v9`, `kafka-go`, `jwt/v5`).

- [ ] **Step 2: Write failing unit tests for `pkg/resilience/circuit_breaker_test.go`**
  Test cases:
  - `TestCircuitBreakerOpensAfterFiveFailures`: 5 consecutive errors within window transition state from `CLOSED` → `OPEN`; 6th call returns `ErrCircuitOpen` without invoking `fn` and completes in `< 20ms`.
  - `TestCircuitBreakerHalfOpenRecovery`: after `ResetTimeout` elapses (using short test timeout e.g. `50ms`), next call runs in `HALF_OPEN`; on success, state returns to `CLOSED`.
  - `TestCircuitBreakerHalfOpenFailureReopens`: if the probe call in `HALF_OPEN` fails, state returns immediately to `OPEN`.
  - `TestCircuitBreakerWindowResetsCount`: failures separated by more than `FailureWindow` do not trip the breaker.

- [ ] **Step 3: Run tests to verify failure**
  Run: `go test ./pkg/resilience/... -v`
  Expected: FAIL (package/symbols undefined).

- [ ] **Step 4: Implement `pkg/resilience/circuit_breaker.go` and `pkg/httputil/problem.go`**
  Implement thread-safe `CircuitBreaker` (`sync.Mutex`) with default constants (`5`, `30s`, `30s`), `Status` snapshot (`State`, `ConsecutiveFailures`, `LastFailureAt`, `OpenedAt`), and `pkg/httputil` RFC 7807 helper matching `06‐Especificaciones‐API.md` §6 (`tipo`, `codigo`, `titulo`, `status`, `detalle`, `instancia`, `requestId`).

- [ ] **Step 5: Run tests to verify pass**
  Run: `go test ./pkg/... -v -race`
  Expected: PASS.

- [ ] **Step 6: Create `db/init.sql` and `api/openapi.yaml`**
  - `db/init.sql`: tables `usuario` (no password column; includes `telefono`, `idioma_pref`, `moneda_pref`), `viaje`, `reserva` (with `idempotency_key UNIQUE`, `fecha_inicio_utc`, `fecha_fin_utc`, `zona_horaria_origen`, `zona_horaria_destino`, `monto`, `moneda_iso`, `detalles_jsonb`), `sesion_chat`, `mensaje_chat`, `ticket_derivacion` (with `idempotency_key UNIQUE`, `estado`, `intentos`, `proximo_reintento_utc`), `idempotency_keys`; seed user `usr_9812a3` ("Nicola Satragni", `+54 9 2494 123456`, `nicola@example.com`), active trip `vj_5501` ("Gira Europea 2026") and flight reservation `res_8819` (Iberia `IB-7412A`, EZE→MAD) + hotel reservation `res_8820`.
  - `api/openapi.yaml`: OpenAPI 3.0.3 contract matching `06‐Especificaciones‐API.md` for all implemented endpoints (`/api/v1/auth/demo-token`, `/api/v1/viajes`, `/api/v1/viajes/{viajeId}/archivar`, `/api/v1/reservas`, `/api/v1/itinerarios/{usuarioId}`, `/api/v1/chatbot/consulta`, `/api/v1/chatbot/tickets/{usuarioId}`, `/api/v1/chatbot/circuit-status`, `/api/v1/agencia/derivacion`).

- [ ] **Step 7: Commit**
  Commit `go.mod`, `go.sum`, `pkg/`, `db/init.sql`, `api/openapi.yaml`.

---

### Task 2: `services/itinerary-service` (Hexagonal Architecture + Redis Cache-Aside + Kafka Publisher)

**Files:**
- Create: `services/itinerary-service/internal/domain/models/models.go`
- Create: `services/itinerary-service/internal/domain/ports/in/use_cases.go`
- Create: `services/itinerary-service/internal/domain/ports/out/ports.go`
- Create: `services/itinerary-service/internal/application/services/itinerary_service.go`
- Test: `services/itinerary-service/internal/application/services/itinerary_service_test.go`
- Create: `services/itinerary-service/internal/infrastructure/adapters/out/postgres/pg_repository.go`
- Create: `services/itinerary-service/internal/infrastructure/adapters/out/redis/redis_cache.go`
- Create: `services/itinerary-service/internal/infrastructure/adapters/out/kafka/kafka_publisher.go`
- Create: `services/itinerary-service/internal/infrastructure/adapters/in/rest_itinerary_controller.go`
- Create: `services/itinerary-service/cmd/main.go`

**Interfaces:**
- Consumes: `pkg/httputil`
- Produces:
  - HTTP API on `:8081`:
    - `GET /health`
    - `GET /api/v1/itinerarios/{usuarioId}` → `200` `ItinerarioConsolidado` (`origenDatos`: `CACHE_HIT` | `CACHE_MISS`)
    - `GET /api/v1/reservas/{reservaId}` → `200` `ReservaContexto` (includes `usuario`, `viaje`, `reserva` for `chatbot-service`)
    - `POST /api/v1/viajes` (requires `Idempotency-Key`) → `201`
    - `POST /api/v1/reservas` (requires `Idempotency-Key`) → `201` (or `409` on key collision with different payload)
    - `PUT /api/v1/viajes/{viajeId}/archivar` → `200`
  - Kafka event published on topic `itinerary.updated` on trip/reservation mutations.

- [ ] **Step 1: Define domain models and hexagonal ports (`in` & `out`)**
  Create `models.go` (`Viaje`, `Reserva`, `ItinerarioConsolidado`, `UsuarioResumen`, `ReservaContexto`), `ports/in/use_cases.go`, and `ports/out/ports.go` (`ReservationRepositoryPort`, `CachePort`, `EventPublisherPort`).

- [ ] **Step 2: Write failing unit tests in `itinerary_service_test.go`**
  Using in-memory mocks of the 3 output ports, test:
  - `TestGetConsolidatedItinerary_CacheMissThenHit`: first call hits repo, populates cache (`TTL = 15m`), returns `CACHE_MISS`; second call returns `CACHE_HIT` without hitting repo.
  - `TestGetConsolidatedItinerary_RedisDownDegradesGracefully`: when `CachePort.Get` returns an error, service falls back to repo and succeeds with `CACHE_MISS`.
  - `TestCreateReservation_InvalidatesCacheAndPublishesEvent`: creating a reservation validates dates (`fechaInicioUtc < fechaFinUtc`), invalidates `itinerario:{usuarioId}` in cache, and publishes `itinerary.updated` event.
  - `TestCreateReservation_IdempotencyConflict`: reusing `Idempotency-Key` with different payload returns `ErrIdempotencyConflict` (mapped to `409`).

- [ ] **Step 3: Run tests to verify failure**
  Run: `go test ./services/itinerary-service/... -v`
  Expected: FAIL.

- [ ] **Step 4: Implement `ItineraryApplicationService` and verify unit tests pass**
  Implement `itinerary_service.go` and run `go test ./services/itinerary-service/... -v -race`.
  Expected: PASS.

- [ ] **Step 5: Implement infrastructure adapters (`postgres`, `redis`, `kafka`, `rest_itinerary_controller`) and `cmd/main.go`**
  Wire `pgxpool`, `go-redis` (`TTL = 15 * time.Minute`), `kafka-go` writer (`itinerary.updated`), `net/http` ServeMux (Go 1.22+ method/path patterns), and graceful shutdown.

- [ ] **Step 6: Verify build and tests**
  Run: `go test ./services/itinerary-service/... -race && go build ./services/itinerary-service/cmd`
  Expected: PASS and clean binary build.

- [ ] **Step 7: Commit**
  Commit `services/itinerary-service/`.

---

### Task 3: `services/agency-mock` (Preferred Travel Agency Simulator with Fault Injection)

**Files:**
- Create: `services/agency-mock/cmd/main.go`
- Test: `services/agency-mock/cmd/main_test.go`

**Interfaces:**
- Consumes: `pkg/httputil`
- Produces:
  - HTTP API on `:8089`:
    - `GET /health`
    - `POST /api/v1/agencia/derivacion` (exact JSON contract of `06` §4.2; deduplicates by `Idempotency-Key` in memory; returns `200 OK` `{ticketDerivacion, estado: "RECIBIDO", agenteAsignado: "Operador_Agencia_04", mensaje}` in `healthy` mode; sleeps `5s` in `slow` mode; returns `500` in `error500` mode).
    - `GET /mock/mode` and `POST /mock/mode` (`{"mode": "healthy" | "slow" | "error500"}`).
    - `GET /mock/tickets` (lists received tickets and total count for E2E verification of zero duplicates).

- [ ] **Step 1: Write failing tests in `services/agency-mock/cmd/main_test.go`**
  Test:
  - `TestAgencyMockHealthyAndIdempotent`: sending the same `Idempotency-Key` twice returns `200 OK` both times and stores only 1 ticket.
  - `TestAgencyMockError500Mode`: switching mode to `error500` causes `/api/v1/agencia/derivacion` to return `500`.

- [ ] **Step 2: Run tests to verify failure**
  Run: `go test ./services/agency-mock/... -v`
  Expected: FAIL.

- [ ] **Step 3: Implement `services/agency-mock/cmd/main.go` and verify tests pass**
  Run: `go test ./services/agency-mock/... -v -race`
  Expected: PASS.

- [ ] **Step 4: Commit**
  Commit `services/agency-mock/`.

---

### Task 4: `services/chatbot-service` (Opción B — Intent Classifier + Circuit Breaker + Outbox Worker)

**Files:**
- Create: `services/chatbot-service/internal/domain/intent_classifier.go`
- Test: `services/chatbot-service/internal/domain/intent_classifier_test.go`
- Create: `services/chatbot-service/internal/infrastructure/adapters/itinerary_client.go`
- Create: `services/chatbot-service/internal/infrastructure/adapters/agency_webhook_client.go`
- Create: `services/chatbot-service/internal/infrastructure/outbox/outbox_repository.go`
- Create: `services/chatbot-service/internal/infrastructure/outbox/outbox_worker.go`
- Test: `services/chatbot-service/internal/infrastructure/outbox/outbox_worker_test.go`
- Create: `services/chatbot-service/internal/infrastructure/adapters/rest_chatbot_controller.go`
- Create: `services/chatbot-service/cmd/main.go`

**Interfaces:**
- Consumes: `pkg/httputil`, `pkg/resilience`, `itinerary-service` (`GET /api/v1/reservas/{reservaId}`, `GET /api/v1/itinerarios/{usuarioId}`), `agency-mock` (`POST /api/v1/agencia/derivacion`).
- Produces:
  - HTTP API on `:8082`:
    - `GET /health`
    - `POST /api/v1/chatbot/consulta` → `200` `ConsultaChatbotResponse` (exact fields of `06` §4.1: `consultaId`, `respuestaBot`, `requiereDerivacion`, `agenciaDestino`, `ticketDerivacion`, `tiempoEstimadoRespuesta`, plus optional `estadoTicket` / `circuitState` metadata).
    - `GET /api/v1/chatbot/tickets/{usuarioId}` → `200` list of `ticket_derivacion` (`PENDIENTE`, `ENTREGADO`, `RESUELTO`).
    - `GET /api/v1/chatbot/circuit-status` → `200` `resilience.Status`.
  - Kafka events on topic `chatbot.events` when a ticket is created or delivered.

- [ ] **Step 1: Write failing tests for `intent_classifier_test.go` and `outbox_worker_test.go`**
  - `intent_classifier_test.go`:
    - Level 1 queries (`"¿Puedo agregar equipaje de mano?"`, `"¿Cuál es el estado de mi vuelo?"`, `"¿A qué hora es el check-in?"`) return `RequiresEscalation: false` with contextual answer using the reservation details.
    - Complex queries (`"Quiero cancelación con reembolso total"`, `"Mi vuelo fue cancelado por fuerza mayor y necesito reprogramar"`, `"Quiero hablar con un agente"`) return `RequiresEscalation: true` with `MotivoDerivacion`.
    - Empty/missing reservation context degrades gracefully without panicking.
  - `outbox_worker_test.go`:
    - Pending ticket fails delivery while agency is down (remains `PENDIENTE`, increments `intentos`); once agency mock returns healthy, next tick delivers with same `Idempotency-Key` and marks ticket `ENTREGADO`.

- [ ] **Step 2: Run tests to verify failure**
  Run: `go test ./services/chatbot-service/... -v`
  Expected: FAIL.

- [ ] **Step 3: Implement `intent_classifier.go`, `outbox_worker.go`, `outbox_repository.go`, HTTP clients (`itinerary_client.go`, `agency_webhook_client.go` with `2.5s` HTTP timeout + `CircuitBreaker`), `rest_chatbot_controller.go`, and `cmd/main.go`**
  - `agency_webhook_client.go`: `http.Client{Timeout: 2500 * time.Millisecond}` wrapped in `resilience.CircuitBreaker` (`FailureThreshold: 5`, `FailureWindow: 30s`, `ResetTimeout: 30s`), sending `Authorization: Bearer <SERVICE_TOKEN>` and `Idempotency-Key`.
  - `outbox_worker.go`: ticks every `5s`, fetches `PENDIENTE` tickets where `proximo_reintento_utc <= NOW()`, retries via `AgencyWebhookClient`, marks `ENTREGADO` on success or schedules exponential backoff (`5s, 10s, 20s, max 60s`).

- [ ] **Step 4: Run tests and build**
  Run: `go test ./services/chatbot-service/... -v -race && go build ./services/chatbot-service/cmd`
  Expected: PASS.

- [ ] **Step 5: Commit**
  Commit `services/chatbot-service/`.

---

### Task 5: `services/web-bff`, Minimal Frontend (`web/`), `Dockerfile`, and `docker-compose.yml`

**Files:**
- Create: `services/web-bff/internal/auth.go`
- Test: `services/web-bff/internal/auth_test.go`
- Create: `services/web-bff/internal/router.go`
- Create: `services/web-bff/cmd/main.go`
- Create: `web/index.html`, `web/styles.css`, `web/app.js`
- Create: `Dockerfile`
- Create: `docker-compose.yml`

**Interfaces:**
- Consumes: `pkg/httputil`, `itinerary-service:8081`, `chatbot-service:8082`, `agency-mock:8089`, `api/openapi.yaml`, `web/`.
- Produces:
  - Public entry point on `:8080`:
    - `POST /api/v1/auth/demo-token` (issues signed `RS256` JWT for demo user `usr_9812a3` or custom `usuarioId`).
    - Authenticated proxy routes (`/api/v1/viajes`, `/api/v1/viajes/{viajeId}/archivar`, `/api/v1/reservas`, `/api/v1/itinerarios/{usuarioId}`, `/api/v1/chatbot/consulta`, `/api/v1/chatbot/tickets/{usuarioId}`, `/api/v1/chatbot/circuit-status`).
    - Demo agency mode proxy: `GET/POST /api/v1/demo/agency-mode` → `agency-mock:8089/mock/mode`.
    - Static UI at `/`, `/openapi.yaml`, and `/docs` (minimal Swagger UI page pointing to `/openapi.yaml`).

- [ ] **Step 1: Write failing tests in `services/web-bff/internal/auth_test.go`**
  Test:
  - Missing `Authorization` header returns `401` RFC 7807.
  - Valid `RS256` JWT for `usr_9812a3` accessing `/api/v1/itinerarios/usr_otro` returns `403` RFC 7807.
  - Valid `RS256` JWT for `usr_9812a3` accessing `/api/v1/itinerarios/usr_9812a3` passes to handler.

- [ ] **Step 2: Run tests to verify failure**
  Run: `go test ./services/web-bff/... -v`
  Expected: FAIL.

- [ ] **Step 3: Implement `web-bff` (`auth.go`, `router.go`, `cmd/main.go`) and verify tests pass**
  Run: `go test ./services/web-bff/... -v -race`
  Expected: PASS.

- [ ] **Step 4: Create minimal frontend (`web/index.html`, `web/styles.css`, `web/app.js`)**
  Clean, responsive two-column layout:
  - Left column: **Panel de Viajes** (active trip + reservations with UTC→local display, `CACHE_HIT`/`CACHE_MISS` badge + latency ms, refresh button, quick form to add a manual reservation, archive button).
  - Right column: **Asistente Virtual (Chatbot — Opción B)** (message feed, quick-prompt buttons for Nivel 1 and Escalación Compleja, live **Circuit Breaker** status badge, **Tickets Outbox** list showing `PENDIENTE` → `ENTREGADO`, and **Agencia Externa** mode selector `Saludable / Lenta 5s / Error 500`).

- [ ] **Step 5: Create `Dockerfile` and `docker-compose.yml`**
  - Single multi-stage `Dockerfile` taking `ARG SERVICE` to build each of the 4 Go binaries cleanly.
  - `docker-compose.yml` running `postgres:16-alpine` (with `db/init.sql` mounted + healthcheck), `redis:7-alpine`, `redpandadata/redpanda:v24.2.10` (single-node, creating `itinerary.updated` and `chatbot.events`), `agency-mock`, `itinerary-service`, `chatbot-service`, and `web-bff`.

- [ ] **Step 6: Verify all Go tests and builds**
  Run: `go test ./... -v -race`
  Expected: all unit tests PASS.

- [ ] **Step 7: Commit**
  Commit `services/web-bff/`, `web/`, `Dockerfile`, `docker-compose.yml`.

---

### Task 6: End-to-End Verification (`demo_e2e.sh`) + Wiki Synchronization

**Files:**
- Create: `scripts/demo_e2e.sh`
- Modify: `README.md` (run instructions for 2da entrega)
- Modify: `trotamundos-dss.wiki/05‐Vistas‐Arquitectonicas.md` (Vista de Módulos `.ts` → `.go` matching implemented layout)
- Modify: `trotamundos-dss.wiki/04‐ADRs.md` (`ADR-013` status `Propuesto` → `Aceptado`)
- Modify: `trotamundos-dss.wiki/03‐Iteraciones‐ADD.md` (update Kanban closure reflecting prototype validation)

**Interfaces:**
- Consumes: running Docker Compose stack on `http://localhost:8080`.
- Produces: reproducible E2E verification script + 100% synced Wiki.

- [ ] **Step 1: Write `scripts/demo_e2e.sh`**
  Automated bash/curl verification testing:
  1. Health checks and `POST /api/v1/auth/demo-token` (`401` without token, `403` cross-user).
  2. `GET /api/v1/itinerarios/usr_9812a3` → first `CACHE_MISS`, second `CACHE_HIT` (`< 800 ms`).
  3. `POST /api/v1/reservas` with `Idempotency-Key` → `201`, invalidates cache; duplicate key with different payload → `409`.
  4. `POST /api/v1/chatbot/consulta` Level 1 → `requiereDerivacion: false` (`< 300 ms`).
  5. `POST /api/v1/chatbot/consulta` Complex (agency `healthy`) → `requiereDerivacion: true`, ticket `ENTREGADO`.
  6. Switch agency to `error500` → send 5 complex queries → verify Circuit Breaker transitions to `OPEN` and subsequent call fails fast in `< 50ms` saving ticket as `PENDIENTE` in Outbox.
  7. Switch agency back to `healthy` → wait for Outbox worker tick → verify tickets transition to `ENTREGADO` with zero duplicate tickets in agency.

- [ ] **Step 2: Build and start Docker Compose stack and run `scripts/demo_e2e.sh`**
  Run: `docker compose up -d --build && ./scripts/demo_e2e.sh`
  Expected: all 7 E2E checks PASS with exit code 0.

- [ ] **Step 3: Sync Wiki (`05`, `04`, `03`) and update `README.md`**
  - Update `05‐Vistas‐Arquitectonicas.md` §2 Vista de Módulos to Go (`.go`) reflecting the real repository structure and noting the acotated prototype scope.
  - Update `04‐ADRs.md` `ADR-013` to `Aceptado` with implementation reference.
  - Update `03‐Iteraciones‐ADD.md` Kanban closure to note validation via the Go prototype.
  - Run internal wiki link verification script to confirm 0 broken links.

- [ ] **Step 4: Run full verification (`go test ./... -race`, E2E check, wiki link check) and commit**
  Expected: 100% green before reporting completion.
