package outbox

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/resilience"
)

type TicketSender interface {
	DeliverTicket(ctx context.Context, idempotencyKey string, payloadJSON []byte) (string, error)
}

type DeliveryObserver func(ctx context.Context, tk TicketRecord, agente string)

type Worker struct {
	store    OutboxStore
	sender   TicketSender
	onSent   DeliveryObserver
	interval time.Duration
}

func NewWorker(store OutboxStore, sender TicketSender, onSent DeliveryObserver) *Worker {
	return &Worker{
		store:    store,
		sender:   sender,
		onSent:   onSent,
		interval: 5 * time.Second,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ProcessOnce(ctx)
		}
	}
}

func (w *Worker) ProcessOnce(ctx context.Context) {
	pending, err := w.store.FetchPendingDue(ctx, 20)
	if err != nil {
		slog.Warn("outbox fetch pending failed", "err", err)
		return
	}
	for _, tk := range pending {
		agente, err := w.sender.DeliverTicket(ctx, tk.IdempotencyKey, tk.PayloadJSON)
		if err != nil {
			if errors.Is(err, resilience.ErrCircuitOpen) {
				slog.Info("outbox paused while circuit breaker is open")
				return
			}
			attempts := tk.Intentos + 1
			backoff := computeBackoff(attempts)
			slog.Warn("outbox retry deferred", "ticketId", tk.TicketID, "attempt", attempts, "backoff", backoff, "err", err)
			_ = w.store.MarkRetry(ctx, tk.TicketID, attempts, time.Now().UTC().Add(backoff), err.Error())
			continue
		}
		if err := w.store.MarkDelivered(ctx, tk.TicketID, agente); err != nil {
			slog.Error("outbox mark delivered failed", "ticketId", tk.TicketID, "err", err)
			continue
		}
		slog.Info("outbox ticket delivered after recovery", "ticketId", tk.TicketID, "agente", agente)
		if w.onSent != nil {
			w.onSent(ctx, tk, agente)
		}
	}
}

func computeBackoff(attempt int) time.Duration {
	d := 5 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= 30*time.Second {
			return 30 * time.Second
		}
	}
	return d
}
