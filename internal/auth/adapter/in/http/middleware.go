package httpadapter

import (
	"errors"
	"net/http"
	"strings"

	authout "go_pet_project/internal/auth/application/port/out"
	"go_pet_project/internal/platform/httpx"
)

type AccessTokenValidator interface {
	ValidateAccessToken(token string) (authout.AccessTokenClaims, error)
}

type AuthMiddleware struct {
	tokens AccessTokenValidator
}

func NewAuthMiddleware(tokens AccessTokenValidator) (*AuthMiddleware, error) {
	if tokens == nil {
		return nil, errors.New("access token validator is required")
	}

	return &AuthMiddleware{tokens: tokens}, nil
}

func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeUnauthorized(w)
			return
		}

		claims, err := m.tokens.ValidateAccessToken(token)
		if err != nil {
			writeUnauthorized(w)
			return
		}

		ctx := contextWithUserID(r.Context(), claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func parseBearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	if parts[1] == "" {
		return "", false
	}

	return parts[1], true
}

func writeUnauthorized(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
}
