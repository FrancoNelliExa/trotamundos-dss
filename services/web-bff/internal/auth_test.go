package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJWTMiddleware_UnauthorizedAndCrossUserForbidden(t *testing.T) {
	auth, err := NewJWTManager()
	if err != nil {
		t.Fatalf("failed to init JWTManager: %v", err)
	}

	protected := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.AuthorizeUser(w, r, r.PathValue("usuarioId")) {
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/itinerarios/{usuarioId}", protected)

	// 1. Missing token -> 401
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/itinerarios/usr_9812a3", nil)
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w1.Code)
	}

	token, err := auth.IssueToken("usr_9812a3")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	// 2. Valid token for usr_9812a3 accessing usr_otro -> 403
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/itinerarios/usr_otro", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-user access, got %d", w2.Code)
	}

	// 3. Valid token for own user -> 200
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/itinerarios/usr_9812a3", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 for authorized user, got %d", w3.Code)
	}
}
