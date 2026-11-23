const USER_ID = 'usr_9812a3';
let authToken = '';
let activeViajeId = 'vj_5501';
let activeReservaId = 'res_8819';

async function apiFetch(path, options = {}) {
  const headers = Object.assign(
    { 'Content-Type': 'application/json', 'Accept-Language': 'es-AR' },
    options.headers || {}
  );
  if (authToken) {
    headers['Authorization'] = `Bearer ${authToken}`;
  }
  const t0 = performance.now();
  const resp = await fetch(path, { ...options, headers });
  const elapsedMs = Math.round(performance.now() - t0);
  const data = await resp.json().catch(() => ({}));
  return { ok: resp.ok, status: resp.status, data, elapsedMs };
}

async function initAuth() {
  const badge = document.getElementById('session-badge');
  const res = await apiFetch('/api/v1/auth/demo-token', {
    method: 'POST',
    body: JSON.stringify({ usuarioId: USER_ID }),
  });
  if (res.ok && res.data.token) {
    authToken = res.data.token;
    badge.textContent = `JWT RS256 activo (${USER_ID})`;
    badge.className = 'badge badge-ok';
  } else {
    badge.textContent = 'Error de autenticación';
    badge.className = 'badge badge-err';
  }
}

async function loadItinerary() {
  const container = document.getElementById('itinerary-container');
  const cacheBadge = document.getElementById('cache-badge');
  const latBadge = document.getElementById('latency-badge');

  const res = await apiFetch(`/api/v1/itinerarios/${USER_ID}`);
  latBadge.textContent = `${res.elapsedMs} ms`;
  latBadge.className = res.elapsedMs < 800 ? 'badge badge-ok' : 'badge badge-warn';

  if (!res.ok) {
    container.textContent = res.data.detalle || 'No se pudo cargar el itinerario.';
    return;
  }

  const origin = res.data.origenDatos || 'CACHE_MISS';
  cacheBadge.textContent = origin;
  cacheBadge.className = origin === 'CACHE_HIT' ? 'badge badge-ok' : 'badge badge-warn';

  container.replaceChildren();
  const trips = res.data.viajesActivos || [];
  if (trips.length === 0) {
    const empty = document.createElement('p');
    empty.className = 'muted';
    empty.textContent = 'No hay viajes activos.';
    container.appendChild(empty);
    return;
  }

  activeViajeId = trips[0].viajeId;
  if (trips[0].reservas && trips[0].reservas.length > 0) {
    activeReservaId = trips[0].reservas[0].reservaId;
  }

  for (const trip of trips) {
    const box = document.createElement('div');
    box.className = 'trip-box';

    const top = document.createElement('div');
    top.className = 'trip-top';
    const title = document.createElement('h3');
    title.textContent = `${trip.nombreViaje} — ${trip.destinoPrincipal}`;
    const dates = document.createElement('span');
    dates.className = 'badge badge-neutral';
    dates.textContent = `${trip.fechaInicio} → ${trip.fechaFin}`;
    top.append(title, dates);
    box.appendChild(top);

    for (const r of trip.reservas || []) {
      const item = document.createElement('div');
      item.className = 'res-item';

      const head = document.createElement('div');
      head.className = 'res-head';
      const left = document.createElement('span');
      left.textContent = `[${r.tipoReserva}] ${r.proveedor} (${r.codigoConfirmacion})`;
      const right = document.createElement('span');
      right.textContent = `${r.monto} ${r.monedaIso}`;
      head.append(left, right);

      const meta = document.createElement('div');
      meta.className = 'muted';
      meta.textContent = `Inicio UTC: ${r.fechaInicioUtc} (${r.zonaHorariaOrigen}) · Estado: ${r.estado}`;
      item.append(head, meta);
      box.appendChild(item);
    }

    container.appendChild(box);
  }
}

function appendChatMessage(sender, text, metaText = '') {
  const log = document.getElementById('chat-log');
  const div = document.createElement('div');
  div.className = sender === 'user' ? 'msg msg-user' : 'msg msg-bot';
  div.textContent = text;
  if (metaText) {
    const meta = document.createElement('span');
    meta.className = 'msg-meta';
    meta.textContent = metaText;
    div.appendChild(meta);
  }
  log.appendChild(div);
  log.scrollTop = log.scrollHeight;
}

async function sendChatMessage(message) {
  appendChatMessage('user', message);
  const res = await apiFetch('/api/v1/chatbot/consulta', {
    method: 'POST',
    body: JSON.stringify({
      usuarioId: USER_ID,
      reservaId: activeReservaId,
      mensajeUsuario: message,
    }),
  });
  if (!res.ok) {
    appendChatMessage('bot', `Error ${res.status}: ${res.data.detalle || 'Fallo al consultar el chatbot'}`);
    return;
  }
  const d = res.data;
  const parts = [`${res.elapsedMs} ms`];
  if (d.requiereDerivacion) {
    parts.push(`Ticket: ${d.ticketDerivacion}`);
    parts.push(`Estado: ${d.estadoTicket}`);
  } else {
    parts.push('Resuelto Nivel 1');
  }
  if (d.circuitState) parts.push(`CB: ${d.circuitState}`);
  appendChatMessage('bot', d.respuestaBot, parts.join(' · '));
  await refreshStatusAndTickets();
}

async function refreshStatusAndTickets() {
  const [cbRes, tkRes, agRes] = await Promise.all([
    apiFetch('/api/v1/chatbot/circuit-status'),
    apiFetch(`/api/v1/chatbot/tickets/${USER_ID}`),
    apiFetch('/api/v1/demo/agency-mode'),
  ]);

  if (cbRes.ok) {
    const st = cbRes.data;
    const badge = document.getElementById('cb-badge');
    badge.textContent = `CB: ${st.state} (${st.consecutiveFailures}/${st.failureThreshold})`;
    badge.className =
      st.state === 'CLOSED' ? 'badge badge-ok' : st.state === 'HALF_OPEN' ? 'badge badge-warn' : 'badge badge-err';
  }

  if (agRes.ok) {
    const mode = agRes.data.mode;
    const labels = { healthy: 'Saludable (200 OK)', slow: 'Lenta (5s > timeout 2.5s)', error500: 'Caída (HTTP 500)' };
    document.getElementById('agency-mode-label').textContent = labels[mode] || mode;
    document.querySelectorAll('[data-agency-mode]').forEach((btn) => {
      btn.classList.toggle('btn-active', btn.dataset.agencyMode === mode);
    });
  }

  if (tkRes.ok) {
    const listEl = document.getElementById('tickets-list');
    const tickets = tkRes.data.tickets || [];
    listEl.replaceChildren();
    if (tickets.length === 0) {
      const p = document.createElement('p');
      p.className = 'muted';
      p.textContent = 'Sin derivaciones registradas todavía.';
      listEl.appendChild(p);
      return;
    }
    for (const tk of tickets) {
      const row = document.createElement('div');
      row.className = 'ticket-row';

      const info = document.createElement('div');
      const strong = document.createElement('strong');
      strong.textContent = `${tk.ticketId} `;
      const reason = document.createElement('span');
      reason.textContent = `— ${tk.motivoDerivacion}`;
      const sub = document.createElement('div');
      sub.className = 'muted';
      sub.textContent = tk.agenteAsignado
        ? `Agente: ${tk.agenteAsignado}`
        : `Reintentos Outbox: ${tk.intentos}${tk.ultimoError ? ' · ' + tk.ultimoError : ''}`;
      info.append(strong, reason, sub);

      const stBadge = document.createElement('span');
      stBadge.className = tk.estado === 'ENTREGADO' ? 'badge badge-ok' : 'badge badge-warn';
      stBadge.textContent = tk.estado;

      row.append(info, stBadge);
      listEl.appendChild(row);
    }
  }
}

document.addEventListener('DOMContentLoaded', async () => {
  await initAuth();
  await loadItinerary();
  await refreshStatusAndTickets();

  document.getElementById('btn-refresh-itin').addEventListener('click', loadItinerary);
  document.getElementById('btn-refresh-tickets').addEventListener('click', refreshStatusAndTickets);

  document.getElementById('chat-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const input = document.getElementById('chat-input');
    const msg = input.value.trim();
    if (!msg) return;
    input.value = '';
    await sendChatMessage(msg);
  });

  document.querySelectorAll('[data-prompt]').forEach((btn) => {
    btn.addEventListener('click', () => sendChatMessage(btn.dataset.prompt));
  });

  document.querySelectorAll('[data-agency-mode]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      await apiFetch('/api/v1/demo/agency-mode', {
        method: 'POST',
        body: JSON.stringify({ mode: btn.dataset.agencyMode }),
      });
      await refreshStatusAndTickets();
    });
  });

  document.getElementById('reservation-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const body = {
      viajeId: activeViajeId,
      tipoReserva: fd.get('tipoReserva'),
      proveedor: fd.get('proveedor'),
      codigoConfirmacion: fd.get('codigoConfirmacion'),
      fechaInicioUtc: '2026-11-16T09:00:00Z',
      fechaFinUtc: '2026-11-16T12:00:00Z',
      zonaHorariaOrigen: 'Europe/Madrid',
      zonaHorariaDestino: 'Europe/Madrid',
      monto: Number(fd.get('monto')),
      monedaIso: fd.get('monedaIso'),
    };
    const res = await apiFetch('/api/v1/reservas', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: JSON.stringify(body),
    });
    if (res.ok) {
      e.target.reset();
      await loadItinerary();
    } else {
      alert(res.data.detalle || 'No se pudo crear la reserva');
    }
  });

  // Refresco liviano cada 5s para ver en vivo cuando la Cola Outbox entrega tickets PENDIENTE -> ENTREGADO
  setInterval(refreshStatusAndTickets, 5000);
});
