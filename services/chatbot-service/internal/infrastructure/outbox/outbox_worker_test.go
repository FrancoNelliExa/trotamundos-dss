package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/resilience"
)

type fakeStore struct {
	tickets   []TicketRecord
	delivered map[string]string
	retried   map[string]int
}

func (s *fakeStore) FetchPendingDue(_ context.Context, _ int) ([]TicketRecord, error) {
	var out []TicketRecord
	for _, tk := range s.tickets {
		if tk.Estado == "PENDIENTE" {
			out = append(out, tk)
		}
	}
	return out, nil
}

func (s *fakeStore) MarkDelivered(_ context.Context, ticketID, agente string) error {
	s.delivered[ticketID] = agente
	for i := range s.tickets {
		if s.tickets[i].TicketID == ticketID {
			s.tickets[i].Estado = "ENTREGADO"
			s.tickets[i].AgenteAsignado = agente
		}
	}
	return nil
}

func (s *fakeStore) MarkRetry(_ context.Context, ticketID string, attempts int, _ time.Time, _ string) error {
	s.retried[ticketID] = attempts
	for i := range s.tickets {
		if s.tickets[i].TicketID == ticketID {
			s.tickets[i].Intentos = attempts
		}
	}
	return nil
}

type fakeSender struct {
	fail        bool
	errToReturn error
	sentIdemKey []string
}

func (f *fakeSender) DeliverTicket(_ context.Context, idempotencyKey string, _ []byte) (string, error) {
	f.sentIdemKey = append(f.sentIdemKey, idempotencyKey)
	if f.errToReturn != nil {
		return "", f.errToReturn
	}
	if f.fail {
		return "", errors.New("agency unavailable")
	}
	return "Operador_Agencia_04", nil
}

func TestOutboxWorker_RetriesAndDeliversOnRecovery(t *testing.T) {
	store := &fakeStore{
		tickets: []TicketRecord{
			{
				TicketID:       "TCK-9901",
				IdempotencyKey: "idem-outbox-1",
				Estado:         "PENDIENTE",
				PayloadJSON:    []byte(`{"ticketDerivacion":"TCK-9901"}`),
			},
		},
		delivered: map[string]string{},
		retried:   map[string]int{},
	}
	sender := &fakeSender{fail: true}
	worker := NewWorker(store, sender, nil)

	// Tick 1: agency down -> stays PENDIENTE, attempts=1
	worker.ProcessOnce(context.Background())
	if store.tickets[0].Estado != "PENDIENTE" || store.retried["TCK-9901"] != 1 {
		t.Fatalf("expected PENDIENTE with 1 attempt, got %+v", store.tickets[0])
	}

	// Agency recovers -> Tick 2 delivers using same Idempotency-Key -> ENTREGADO
	sender.fail = false
	worker.ProcessOnce(context.Background())
	if store.tickets[0].Estado != "ENTREGADO" || store.delivered["TCK-9901"] != "Operador_Agencia_04" {
		t.Fatalf("expected ENTREGADO after recovery, got %+v", store.tickets[0])
	}
	if len(sender.sentIdemKey) != 2 || sender.sentIdemKey[0] != "idem-outbox-1" || sender.sentIdemKey[1] != "idem-outbox-1" {
		t.Fatalf("expected identical idempotency key on retry, got %v", sender.sentIdemKey)
	}
}

func TestOutboxWorker_CircuitOpenDoesNotBurnAttemptsOrBackoff(t *testing.T) {
	store := &fakeStore{
		tickets: []TicketRecord{
			{TicketID: "TCK-1", IdempotencyKey: "k1", Estado: "PENDIENTE", PayloadJSON: []byte(`{}`)},
			{TicketID: "TCK-2", IdempotencyKey: "k2", Estado: "PENDIENTE", PayloadJSON: []byte(`{}`)},
		},
		delivered: map[string]string{},
		retried:   map[string]int{},
	}
	sender := &fakeSender{errToReturn: resilience.ErrCircuitOpen}
	worker := NewWorker(store, sender, nil)

	worker.ProcessOnce(context.Background())

	if len(store.retried) != 0 {
		t.Fatalf("expected 0 MarkRetry calls while circuit is OPEN, got %v", store.retried)
	}
	if len(sender.sentIdemKey) != 1 {
		t.Fatalf("expected worker to stop batch immediately on ErrCircuitOpen, got %d calls", len(sender.sentIdemKey))
	}
}
