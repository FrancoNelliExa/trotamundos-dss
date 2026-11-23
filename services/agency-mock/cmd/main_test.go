package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgencyMockHealthyAndIdempotent(t *testing.T) {
	srv := NewAgencyServer()

	body := `{"ticketDerivacion":"TCK-100","idempotencyKey":"idem-abc","motivoDerivacion":"Reembolso"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agencia/derivacion", bytes.NewBufferString(body))
		req.Header.Set("Idempotency-Key", "idem-abc")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("iter %d: expected 200 OK, got %d (%s)", i, w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["estado"] != "RECIBIDO" {
			t.Fatalf("expected RECIBIDO, got %v", resp["estado"])
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/mock/tickets", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var list struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if list.Total != 1 {
		t.Fatalf("expected 1 deduplicated ticket, got %d", list.Total)
	}
}

func TestAgencyMockError500Mode(t *testing.T) {
	srv := NewAgencyServer()

	modeReq := httptest.NewRequest(http.MethodPost, "/mock/mode", bytes.NewBufferString(`{"mode":"error500"}`))
	mw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mw, modeReq)
	if mw.Code != http.StatusOK {
		t.Fatalf("expected 200 changing mode, got %d", mw.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agencia/derivacion", bytes.NewBufferString(`{"ticketDerivacion":"TCK-200"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 in error500 mode, got %d", w.Code)
	}
}
