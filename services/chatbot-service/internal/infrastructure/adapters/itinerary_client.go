package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/services/chatbot-service/internal/domain"
)

var ErrForbiddenReservation = errors.New("reservation belongs to another user")

type UserContext struct {
	UsuarioID string `json:"usuarioId"`
	Nombre    string `json:"nombre"`
	Email     string `json:"email"`
	Telefono  string `json:"telefono"`
}

type ReservationContextResponse struct {
	Usuario UserContext            `json:"usuario"`
	Reserva domain.ReservationInfo `json:"reserva"`
}

type ItineraryClient struct {
	baseURL string
	client  *http.Client
}

func NewItineraryClient(baseURL string) *ItineraryClient {
	return &ItineraryClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 2 * time.Second},
	}
}

func (c *ItineraryClient) FetchReservationContext(ctx context.Context, usuarioID, reservaID string) (*ReservationContextResponse, error) {
	if reservaID != "" {
		endpoint := c.baseURL + "/api/v1/reservas/" + url.PathEscape(reservaID) + "?usuarioId=" + url.QueryEscape(usuarioID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden {
			return nil, ErrForbiddenReservation
		}
		if resp.StatusCode == http.StatusOK {
			var out ReservationContextResponse
			if err := json.NewDecoder(resp.Body).Decode(&out); err == nil {
				return &out, nil
			}
		}
	}

	// Fallback: lookup traveler's active itinerary and pick first reservation if present
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/itinerarios/"+usuarioID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itinerary service status %d", resp.StatusCode)
	}
	var itin struct {
		ViajesActivos []struct {
			Reservas []domain.ReservationInfo `json:"reservas"`
		} `json:"viajesActivos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&itin); err != nil {
		return nil, err
	}
	for _, v := range itin.ViajesActivos {
		if len(v.Reservas) > 0 {
			firstID := v.Reservas[0].ReservaID
			if firstID != "" && firstID != reservaID {
				return c.FetchReservationContext(ctx, usuarioID, firstID)
			}
		}
	}
	return nil, nil
}
