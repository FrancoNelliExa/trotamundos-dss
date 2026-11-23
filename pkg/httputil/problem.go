package httputil

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// ProblemDetails sigue el estándar RFC 7807 definido en 06-Especificaciones-API §6.
type ProblemDetails struct {
	Tipo      string `json:"tipo"`
	Codigo    string `json:"codigo"`
	Titulo    string `json:"titulo"`
	Status    int    `json:"status"`
	Detalle   string `json:"detalle"`
	Instancia string `json:"instancia"`
	RequestID string `json:"requestId"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteProblem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ProblemDetails{
		Tipo:      "https://trotamundos.com/errores/" + code,
		Codigo:    code,
		Titulo:    title,
		Status:    status,
		Detalle:   detail,
		Instancia: r.URL.Path,
		RequestID: RequestID(r),
	})
}

func RequestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	return NewID("req")
}

func NewID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, Accept-Language, X-Request-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
