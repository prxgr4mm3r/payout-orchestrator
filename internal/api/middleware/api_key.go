package middleware

import (
	"context"
	"errors"
	"net/http"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/api/auth"
	authservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/auth"
)

const apiKeyHeader = "X-API-Key"

type Authenticator interface {
	AuthenticateAPIKey(ctx context.Context, rawAPIKey string) (apiauth.Client, error)
}

func APIKey(authenticator Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil {
				http.Error(w, "auth middleware is not configured", http.StatusInternalServerError)
				return
			}

			apiKey := r.Header.Get(apiKeyHeader)
			if apiKey == "" {
				http.Error(w, "missing api key", http.StatusUnauthorized)
				return
			}

			client, err := authenticator.AuthenticateAPIKey(r.Context(), apiKey)
			if err != nil {
				switch {
				case errors.Is(err, authservice.ErrInvalidAPIKey), errors.Is(err, authservice.ErrUnauthorized):
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				default:
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}

				return
			}

			next.ServeHTTP(w, r.WithContext(apiauth.WithClient(r.Context(), client)))
		})
	}
}
