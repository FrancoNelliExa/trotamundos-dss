package resilience

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerOpensAfterFiveFailures(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 5,
		FailureWindow:    30 * time.Second,
		ResetTimeout:     30 * time.Second,
	})

	boom := errors.New("agency error 500")
	for i := 0; i < 5; i++ {
		if cb.State() != StateClosed {
			t.Fatalf("expected CLOSED before threshold (iter %d), got %s", i, cb.State())
		}
		err := cb.Execute(func() error { return boom })
		if !errors.Is(err, boom) {
			t.Fatalf("expected boom error, got %v", err)
		}
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN after 5 consecutive failures, got %s", cb.State())
	}

	called := false
	start := time.Now()
	err := cb.Execute(func() error {
		called = true
		return nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if called {
		t.Fatalf("fn should not have been called while circuit is OPEN")
	}
	if elapsed > 20*time.Millisecond {
		t.Fatalf("expected fail-fast < 20ms, took %v", elapsed)
	}
}

func TestCircuitBreakerHalfOpenRecovery(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 2,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
	})

	boom := errors.New("timeout")
	_ = cb.Execute(func() error { return boom })
	_ = cb.Execute(func() error { return boom })
	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN, got %s", cb.State())
	}

	time.Sleep(65 * time.Millisecond)

	if err := cb.Execute(func() error { return nil }); err != nil {
		t.Fatalf("expected probe in HALF_OPEN to succeed, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected CLOSED after successful HALF_OPEN probe, got %s", cb.State())
	}
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 2,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     40 * time.Millisecond,
	})

	boom := errors.New("still down")
	_ = cb.Execute(func() error { return boom })
	_ = cb.Execute(func() error { return boom })
	time.Sleep(55 * time.Millisecond)

	err := cb.Execute(func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error on probe, got %v", err)
	}
	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN again after failed HALF_OPEN probe, got %s", cb.State())
	}
}

func TestCircuitBreakerWindowResetsCount(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 3,
		FailureWindow:    40 * time.Millisecond,
		ResetTimeout:     1 * time.Second,
	})

	boom := errors.New("transient")
	_ = cb.Execute(func() error { return boom })
	_ = cb.Execute(func() error { return boom })
	time.Sleep(55 * time.Millisecond)
	_ = cb.Execute(func() error { return boom })

	if cb.State() != StateClosed {
		t.Fatalf("expected CLOSED when failures fall outside window, got %s", cb.State())
	}
}
