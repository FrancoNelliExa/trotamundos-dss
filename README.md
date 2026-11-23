# Trotamundos — Prototipo Ejecutable en Go (2da Entrega DSS)

Trabajo Práctico Especial de **Diseño de Sistemas de Software (UNICEN)** — Nicola Satragni y Franco Nelli.

Este repositorio contiene el prototipo acotado en **Go** centrado en el módulo avanzado **Opción B (Asistente Virtual / Chatbot)** integrado con el núcleo hexagonal de itinerarios, respetando la arquitectura documentada en la [Wiki del proyecto](trotamundos-dss.wiki/Home.md) y los contratos **Spec-driven** (`api/openapi.yaml`).

## Arquitectura del Prototipo

| Servicio | Rol | Patrones / ADRs demostrados |
| :--- | :--- | :--- |
| **`web-bff`** (`:8080`) | Punto de entrada único, JWT `RS256`, UI web mínima y Swagger `/docs` | `ADR-004` (BFF), `ADR-008` (JWT), RFC 7807 |
| **`itinerary-service`** | Gestión de viajes y reservas en Arquitectura Hexagonal | `ADR-005` (PostgreSQL), `ADR-006` (Redis Cache-Aside), `ADR-002` (Kafka), `ADR-010` (Hexagonal) |
| **`chatbot-service`** | **Módulo Avanzado Opción B:** Nivel 1 + derivación contextual | `ADR-013` (Chatbot autónomo), `ADR-014` (Circuit Breaker `2.5s`/5 fallos + Cola Outbox) |
| **`agency-mock`** | Simulador de Agencia Preferida con inyección de fallas en vivo | `POST /api/v1/agencia/derivacion` idempotente + modos `healthy` / `slow` / `error500` |
| **Infraestructura** | **PostgreSQL 16** (`:5433`), **Redis 7** (`:6380`), **Redpanda / Kafka** (`:19092`) | Topología fiel al diagrama C&C |

## Cómo ejecutarlo

```bash
# 1. Levantar toda la plataforma (Postgres + Redis + Redpanda + 4 servicios Go)
docker compose up -d --build

# 2. Abrir en el navegador:
#    - Panel web + Chatbot + control de fallas: http://localhost:8080/
#    - Documentación OpenAPI 3.0 (Swagger UI):  http://localhost:8080/docs

# 3. Correr pruebas unitarias en Go
go test ./... -v -race

# 4. Correr verificación end-to-end automatizada contra Docker Compose
chmod +x scripts/demo_e2e.sh && ./scripts/demo_e2e.sh

# 5. Apagar contenedores
docker compose down -v
```
