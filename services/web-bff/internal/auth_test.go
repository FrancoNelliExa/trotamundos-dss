package internal

import (
	"bytes"
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

func TestRouter_PostViajesBlocksCrossUserIDORAndSetsAuthHeader(t *testing.T) {
	var seenAuthUser string
	itinBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuthUser = r.Header.Get("X-Authenticated-User")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"viajeId":"vj_1"}`))
	}))
	defer itinBackend.Close()

	auth, err := NewJWTManager()
	if err != nil {
		t.Fatalf("init auth: %v", err)
	}
	router, err := NewRouter(auth, RouterConfig{
		ItineraryURL: itinBackend.URL,
		ChatbotURL:   itinBackend.URL,
		AgencyURL:    itinBackend.URL,
		WebDir:       ".",
		OpenAPIPath:  ".",
	})
	if err != nil {
		t.Fatalf("init router: %v", err)
	}

	token, _ := auth.IssueToken("usr_atacante")

	// Cross-user POST /api/v1/viajes -> must be 403 Forbidden
	reqForbidden := httptest.NewRequest(http.MethodPost, "/api/v1/viajes", bytes.NewBufferString(`{"usuarioId":"usr_9812a3","nombreViaje":"Hack"}`))
	reqForbidden.Header.Set("Authorization", "Bearer "+token)
	wForbidden := httptest.NewRecorder()
	router.ServeHTTP(wForbidden, reqForbidden)
	if wForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on cross-user POST /api/v1/viajes, got %d", wForbidden.Code)
	}

	// Own-user POST /api/v1/viajes -> 201 and X-Authenticated-User injected from JWT (overwriting spoofed header)
	reqOK := httptest.NewRequest(http.MethodPost, "/api/v1/viajes", bytes.NewBufferString(`{"usuarioId":"usr_atacante","nombreViaje":"Propio"}`))
	reqOK.Header.Set("Authorization", "Bearer "+token)
	reqOK.Header.Set("X-Authenticated-User", "usr_9812a3")
	wOK := httptest.NewRecorder()
	router.ServeHTTP(wOK, reqOK)
	if wOK.Code != http.StatusCreated {
		t.Fatalf("expected 201 for own trip, got %d", wOK.Code)
	}
	if seenAuthUser != "usr_atacante" {
		t.Fatalf("expected X-Authenticated-User=usr_atacante from JWT, got %q", seenAuthUser)
	}
}
