package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/resilience"
)

type AgencyWebhookClient struct {
	baseURL      string
	serviceToken string
	httpClient   *http.Client
	cb           *resilience.CircuitBreaker
}

func NewAgencyWebhookClient(baseURL, serviceToken string, cb *resilience.CircuitBreaker) *AgencyWebhookClient {
	return &AgencyWebhookClient{
		baseURL:      baseURL,
		serviceToken: serviceToken,
		httpClient:   &http.Client{Timeout: 2500 * time.Millisecond}, // ADR-014: timeout agresivo 2.5s
		cb:           cb,
	}
}

func (c *AgencyWebhookClient) CircuitStatus() resilience.Status {
	return c.cb.Snapshot()
}

func (c *AgencyWebhookClient) DeliverTicket(ctx context.Context, idempotencyKey string, payloadJSON []byte) (string, error) {
	var agente string
	err := c.cb.Execute(func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agencia/derivacion", bytes.NewReader(payloadJSON))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.serviceToken)
		req.Header.Set("Idempotency-Key", idempotencyKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("agency webhook returned HTTP %d", resp.StatusCode)
		}
		var body struct {
			AgenteAsignado string `json:"agenteAsignado"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		agente = body.AgenteAsignado
		if agente == "" {
			agente = "Operador_Agencia_04"
		}
		return nil
	})
	return agente, err
}
