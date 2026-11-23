package resilience

import (
	"errors"
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "CLOSED"
	StateOpen     State = "OPEN"
	StateHalfOpen State = "HALF_OPEN"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type Config struct {
	FailureThreshold int
	FailureWindow    time.Duration
	ResetTimeout     time.Duration
}

type Status struct {
	State               State      `json:"state"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	FailureThreshold    int        `json:"failureThreshold"`
	ResetTimeoutSeconds int        `json:"resetTimeoutSeconds"`
	LastFailureAt       *time.Time `json:"lastFailureAt,omitempty"`
	OpenedAt            *time.Time `json:"openedAt,omitempty"`
}

type CircuitBreaker struct {
	mu                  sync.Mutex
	cfg                 Config
	state               State
	consecutiveFailures int
	windowStart         time.Time
	lastFailureAt       *time.Time
	openedAt            *time.Time
}

func NewCircuitBreaker(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.FailureWindow <= 0 {
		cfg.FailureWindow = 30 * time.Second
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	return &CircuitBreaker{
		cfg:   cfg,
		state: StateClosed,
	}
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.refreshStateLocked(time.Now())
	return cb.state
}

func (cb *CircuitBreaker) Snapshot() Status {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.refreshStateLocked(time.Now())
	return Status{
		State:               cb.state,
		ConsecutiveFailures: cb.consecutiveFailures,
		FailureThreshold:    cb.cfg.FailureThreshold,
		ResetTimeoutSeconds: int(cb.cfg.ResetTimeout.Seconds()),
		LastFailureAt:       cb.lastFailureAt,
		OpenedAt:            cb.openedAt,
	}
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	now := time.Now()
	cb.refreshStateLocked(now)
	if cb.state == StateOpen {
		cb.mu.Unlock()
		return ErrCircuitOpen
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	now = time.Now()
	if err == nil {
		cb.state = StateClosed
		cb.consecutiveFailures = 0
		cb.openedAt = nil
		return nil
	}

	t := now
	cb.lastFailureAt = &t
	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.openedAt = &t
		return err
	}

	if cb.consecutiveFailures == 0 || now.Sub(cb.windowStart) > cb.cfg.FailureWindow {
		cb.windowStart = now
		cb.consecutiveFailures = 1
	} else {
		cb.consecutiveFailures++
	}

	if cb.consecutiveFailures >= cb.cfg.FailureThreshold {
		cb.state = StateOpen
		cb.openedAt = &t
	}
	return err
}

func (cb *CircuitBreaker) refreshStateLocked(now time.Time) {
	if cb.state == StateOpen && cb.openedAt != nil && now.Sub(*cb.openedAt) >= cb.cfg.ResetTimeout {
		cb.state = StateHalfOpen
	}
}
