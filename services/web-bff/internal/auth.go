package internal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"strings"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/pkg/httputil"
	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const ctxUserIDKey ctxKey = "usuarioId"

type JWTManager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

func NewJWTManager() (*JWTManager, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &JWTManager{
		privateKey: key,
		publicKey:  &key.PublicKey,
	}, nil
}

func (m *JWTManager) IssueToken(usuarioID string) (string, error) {
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   usuarioID,
		Issuer:    "trotamundos-web-bff",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return tok.SignedString(m.privateKey)
}

func (m *JWTManager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			httputil.WriteProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Token de autenticación ausente o inválido.", "Debe incluir la cabecera Authorization: Bearer <JWT>.")
			return
		}
		raw := strings.TrimPrefix(authz, "Bearer ")
		var claims jwt.RegisteredClaims
		tok, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
			if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
				return nil, jwt.ErrSignatureInvalid
			}
			return m.publicKey, nil
		})
		if err != nil || !tok.Valid || claims.Subject == "" {
			httputil.WriteProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Token JWT inválido o expirado.", "Solicite un nuevo token en POST /api/v1/auth/demo-token.")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserIDKey, claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *JWTManager) UserIDFromContext(r *http.Request) string {
	v, _ := r.Context().Value(ctxUserIDKey).(string)
	return v
}

func (m *JWTManager) AuthorizeUser(w http.ResponseWriter, r *http.Request, targetUserID string) bool {
	sub := m.UserIDFromContext(r)
	if sub == "" || sub != targetUserID {
		httputil.WriteProblem(w, r, http.StatusForbidden, "FORBIDDEN", "Acceso denegado.", "No tiene permisos para acceder a recursos de otro viajero.")
		return false
	}
	return true
}
