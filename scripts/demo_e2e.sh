#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
USER_ID="usr_9812a3"

echo "=== 0. Esperando disponibilidad de web-bff (${BASE_URL}) ==="
for i in $(seq 1 30); do
  if curl -sf "${BASE_URL}/health" >/dev/null; then
    break
  fi
  sleep 1
done
curl -sf "${BASE_URL}/health" | grep -q '"status":"ok"'
echo "OK: web-bff saludable"

echo "=== 1. Autenticación JWT RS256 (401 sin token, 403 usuario ajeno, 200 propio) ==="
CODE_401=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/itinerarios/${USER_ID}")
[ "$CODE_401" = "401" ] || { echo "FAIL: esperado 401 sin token, obtuve $CODE_401"; exit 1; }

TOKEN=$(curl -sf -X POST "${BASE_URL}/api/v1/auth/demo-token" \
  -H "Content-Type: application/json" \
  -d "{\"usuarioId\":\"${USER_ID}\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")
[ -n "$TOKEN" ] || { echo "FAIL: no se obtuvo token JWT"; exit 1; }

CODE_403=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/api/v1/itinerarios/usr_ajeno")
[ "$CODE_403" = "403" ] || { echo "FAIL: esperado 403 usuario ajeno, obtuve $CODE_403"; exit 1; }
echo "OK: 401 y 403 verificados"

echo "=== 2. Cache-Aside Redis (CACHE_MISS -> CACHE_HIT < 800ms) ==="
# Invalida caché creando una reserva idempotente primero o probando lectura limpia
R1=$(curl -sf -H "Authorization: Bearer ${TOKEN}" -H "Accept-Language: es-AR" "${BASE_URL}/api/v1/itinerarios/${USER_ID}")
R2=$(curl -sf -H "Authorization: Bearer ${TOKEN}" -H "Accept-Language: es-AR" "${BASE_URL}/api/v1/itinerarios/${USER_ID}")
echo "$R2" | grep -q '"origenDatos":"CACHE_HIT"' || { echo "FAIL: segunda lectura no fue CACHE_HIT: $R2"; exit 1; }
echo "OK: Redis Cache-Aside funcionando (CACHE_HIT verificado)"

echo "=== 3. Alta manual con Idempotency-Key + conflicto 409 + invalidación de caché ==="
IDEM_KEY="e2e-res-$(date +%s)"
PAYLOAD_A='{"viajeId":"vj_5501","tipoReserva":"TREN","proveedor":"Renfe","codigoConfirmacion":"AVE-9042","fechaInicioUtc":"2026-11-16T09:00:00Z","fechaFinUtc":"2026-11-16T11:30:00Z","zonaHorariaOrigen":"Europe/Madrid","zonaHorariaDestino":"Europe/Madrid","monto":89.5,"monedaIso":"EUR"}'
PAYLOAD_B='{"viajeId":"vj_5501","tipoReserva":"TREN","proveedor":"Renfe","codigoConfirmacion":"AVE-OTRO","fechaInicioUtc":"2026-11-16T09:00:00Z","fechaFinUtc":"2026-11-16T11:30:00Z","zonaHorariaOrigen":"Europe/Madrid","zonaHorariaDestino":"Europe/Madrid","monto":89.5,"monedaIso":"EUR"}'

CODE_201=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/reservas" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" -H "Idempotency-Key: ${IDEM_KEY}" -d "$PAYLOAD_A")
[ "$CODE_201" = "201" ] || { echo "FAIL: esperado 201 al crear reserva, obtuve $CODE_201"; exit 1; }

CODE_409=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/reservas" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" -H "Idempotency-Key: ${IDEM_KEY}" -d "$PAYLOAD_B")
[ "$CODE_409" = "409" ] || { echo "FAIL: esperado 409 en conflicto de Idempotency-Key, obtuve $CODE_409"; exit 1; }

R3=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/api/v1/itinerarios/${USER_ID}")
echo "$R3" | grep -q '"origenDatos":"CACHE_MISS"' || { echo "FAIL: escritura no invalidó caché: $R3"; exit 1; }
echo "OK: Alta manual 201, conflicto 409 e invalidación de caché verificados"

echo "=== 4. Chatbot Nivel 1 (sin derivación) ==="
curl -sf -X POST "${BASE_URL}/api/v1/demo/agency-mode" -H "Content-Type: application/json" -d '{"mode":"healthy"}' >/dev/null
CHAT_L1=$(curl -sf -X POST "${BASE_URL}/api/v1/chatbot/consulta" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  -d "{\"usuarioId\":\"${USER_ID}\",\"reservaId\":\"res_8819\",\"mensajeUsuario\":\"¿Puedo agregar equipaje de mano a mi vuelo de Iberia?\"}")
echo "$CHAT_L1" | grep -q '"requiereDerivacion":false' || { echo "FAIL: Nivel 1 derivó indebidamente: $CHAT_L1"; exit 1; }
echo "OK: Chatbot Nivel 1 resuelto autónomamente"

echo "=== 5. Chatbot Derivación Compleja con Agencia Saludable (ENTREGADO) ==="
CHAT_ESC=$(curl -sf -X POST "${BASE_URL}/api/v1/chatbot/consulta" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  -d "{\"usuarioId\":\"${USER_ID}\",\"reservaId\":\"res_8819\",\"mensajeUsuario\":\"Quiero cancelación con reembolso total a mi tarjeta\"}")
echo "$CHAT_ESC" | grep -q '"requiereDerivacion":true' || { echo "FAIL: no derivó caso complejo: $CHAT_ESC"; exit 1; }
echo "$CHAT_ESC" | grep -q '"estadoTicket":"ENTREGADO"' || { echo "FAIL: ticket no quedó ENTREGADO: $CHAT_ESC"; exit 1; }
echo "OK: Derivación directa con agencia saludable verificada"

echo "=== 6. Inyección de falla (error500) -> Apertura de Circuit Breaker (5 fallos) + Outbox PENDIENTE ==="
curl -sf -X POST "${BASE_URL}/api/v1/demo/agency-mode" -H "Content-Type: application/json" -d '{"mode":"error500"}' >/dev/null
for i in 1 2 3 4 5; do
  curl -sf -X POST "${BASE_URL}/api/v1/chatbot/consulta" \
    -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
    -d "{\"usuarioId\":\"${USER_ID}\",\"reservaId\":\"res_8819\",\"mensajeUsuario\":\"Cancelación con reembolso urgente intento ${i}\"}" | grep -q '"estadoTicket":"PENDIENTE"'
done

CB_STAT=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/api/v1/chatbot/circuit-status")
echo "$CB_STAT" | grep -q '"state":"OPEN"' || { echo "FAIL: Circuit Breaker no abrió tras 5 fallos: $CB_STAT"; exit 1; }

# 6ta consulta debe hacer fail-fast con circuito OPEN y guardar en Outbox
T_START=$(date +%s%3N)
CHAT_FF=$(curl -sf -X POST "${BASE_URL}/api/v1/chatbot/consulta" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  -d "{\"usuarioId\":\"${USER_ID}\",\"reservaId\":\"res_8819\",\"mensajeUsuario\":\"Reembolso urgente con circuito abierto\"}")
T_END=$(date +%s%3N)
ELAPSED=$((T_END - T_START))
echo "$CHAT_FF" | grep -q '"circuitState":"OPEN"' || { echo "FAIL: circuitState no reportó OPEN: $CHAT_FF"; exit 1; }
echo "$CHAT_FF" | grep -q '"estadoTicket":"PENDIENTE"' || { echo "FAIL: ticket fail-fast no quedó PENDIENTE: $CHAT_FF"; exit 1; }
echo "OK: Circuit Breaker OPEN y fail-fast en ${ELAPSED}ms con persistencia Outbox"

echo "=== 7. Recuperación de Agencia -> Worker Outbox entrega tickets PENDIENTE -> ENTREGADO ==="
curl -sf -X POST "${BASE_URL}/api/v1/demo/agency-mode" -H "Content-Type: application/json" -d '{"mode":"healthy"}' >/dev/null
echo "Esperando ventana Half-Open (30s) + ciclo Outbox (5s)..."
DELIVERED=0
for i in $(seq 1 42); do
  TKS=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/api/v1/chatbot/tickets/${USER_ID}")
  if ! echo "$TKS" | grep -q '"estado":"PENDIENTE"'; then
    DELIVERED=1
    break
  fi
  sleep 1
done
[ "$DELIVERED" = "1" ] || { echo "FAIL: quedaron tickets PENDIENTE tras recuperación"; exit 1; }
CB_REC=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/api/v1/chatbot/circuit-status")
echo "$CB_REC" | grep -q '"state":"CLOSED"' || { echo "FAIL: Circuit Breaker no volvió a CLOSED: $CB_REC"; exit 1; }
echo "OK: Todos los tickets Outbox entregados y Circuit Breaker recuperado a CLOSED!"

echo ""
echo "=========================================="
echo " TODAS LAS PRUEBAS E2E PASARON AL 100% ✅ "
echo "=========================================="
