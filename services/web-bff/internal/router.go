package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	pkghttp "github.com/FrancoNelliExa/trotamundos-dss/pkg/httputil"
)

type RouterConfig struct {
	ItineraryURL string
	ChatbotURL   string
	AgencyURL    string
	WebDir       string
	OpenAPIPath  string
}

func NewRouter(auth *JWTManager, cfg RouterConfig) (http.Handler, error) {
	itinProxy, err := newReverseProxy(cfg.ItineraryURL)
	if err != nil {
		return nil, err
	}
	chatProxy, err := newReverseProxy(cfg.ChatbotURL)
	if err != nil {
		return nil, err
	}
	agencyProxy, err := newReverseProxy(cfg.AgencyURL)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		pkghttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "web-bff"})
	})

	// Emisión de JWT RS256 para demo (sin almacenar contraseñas - CON-02)
	mux.HandleFunc("POST /api/v1/auth/demo-token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UsuarioID string `json:"usuarioId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.TrimSpace(body.UsuarioID) == "" {
			body.UsuarioID = "usr_9812a3"
		}
		tok, err := auth.IssueToken(body.UsuarioID)
		if err != nil {
			pkghttp.WriteProblem(w, r, http.StatusInternalServerError, "TOKEN_ERROR", "No se pudo emitir el token.", err.Error())
			return
		}
		pkghttp.WriteJSON(w, http.StatusOK, map[string]any{
			"token":            tok,
			"usuarioId":        body.UsuarioID,
			"expiresInSeconds": 7200,
		})
	})

	// OpenAPI & Swagger UI
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(cfg.OpenAPIPath)
		if err != nil {
			pkghttp.WriteProblem(w, r, http.StatusNotFound, "NOT_FOUND", "Especificación OpenAPI no encontrada.", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerHTML))
	})

	// Control demo de la Agencia Externa (para probar Circuit Breaker + Outbox en vivo)
	mux.HandleFunc("GET /api/v1/demo/agency-mode", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/mock/mode"
		agencyProxy.ServeHTTP(w, r)
	})
	mux.HandleFunc("POST /api/v1/demo/agency-mode", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/mock/mode"
		agencyProxy.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /api/v1/demo/agency-tickets", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/mock/tickets"
		agencyProxy.ServeHTTP(w, r)
	})

	// Rutas autenticadas hacia itinerary-service
	mux.Handle("GET /api/v1/itinerarios/{usuarioId}", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.AuthorizeUser(w, r, r.PathValue("usuarioId")) {
			return
		}
		itinProxy.ServeHTTP(w, r)
	})))

	mux.Handle("POST /api/v1/viajes", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		itinProxy.ServeHTTP(w, r)
	})))

	mux.Handle("PUT /api/v1/viajes/{viajeId}/archivar", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		itinProxy.ServeHTTP(w, r)
	})))

	mux.Handle("POST /api/v1/reservas", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		itinProxy.ServeHTTP(w, r)
	})))

	// Rutas autenticadas hacia chatbot-service (Opción B)
	mux.Handle("POST /api/v1/chatbot/consulta", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			pkghttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "No se pudo leer el cuerpo de la solicitud.", err.Error())
			return
		}
		var probe struct {
			UsuarioID string `json:"usuarioId"`
		}
		_ = json.Unmarshal(raw, &probe)
		if probe.UsuarioID != "" && !auth.AuthorizeUser(w, r, probe.UsuarioID) {
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		r.ContentLength = int64(len(raw))
		chatProxy.ServeHTTP(w, r)
	})))

	mux.Handle("GET /api/v1/chatbot/tickets/{usuarioId}", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.AuthorizeUser(w, r, r.PathValue("usuarioId")) {
			return
		}
		chatProxy.ServeHTTP(w, r)
	})))

	mux.Handle("GET /api/v1/chatbot/circuit-status", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatProxy.ServeHTTP(w, r)
	})))

	// Frontend mínimo estático (HTML/CSS/JS)
	fileServer := http.FileServer(http.Dir(cfg.WebDir))
	mux.Handle("GET /", fileServer)

	return pkghttp.WithCORS(mux), nil
}

func newReverseProxy(rawURL string) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return httputil.NewSingleHostReverseProxy(u), nil
}

const swaggerHTML = `<!doctype html>
<html lang="es">
<head>
  <meta charset="utf-8" />
  <title>Trotamundos — OpenAPI 3.0 Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => SwaggerUIBundle({ url: '/openapi.yaml', dom_id: '#swagger-ui' });
  </script>
</body>
</html>`
