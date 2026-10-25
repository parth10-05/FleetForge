package controlhttp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
)

type principalKey struct{}

func principalFromContext(ctx context.Context) (access.Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(access.Principal)
	return principal, ok
}

func withAuthentication(verifier access.TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			token, ok := bearerToken(request.Header.Get("Authorization"))
			if !ok {
				writeUnauthorized(writer, correlationID(request.Context()))
				return
			}
			principal, err := verifier.VerifyPrincipal(request.Context(), token)
			if err != nil {
				writeUnauthorized(writer, correlationID(request.Context()))
				return
			}
			next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), principalKey{}, principal)))
		})
	}
}

func requirePermission(permission access.Permission, now func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			principal, ok := principalFromContext(request.Context())
			if !ok || access.Authorize(principal, permission, now()) != nil {
				writeProblem(writer, Problem{
					Type:          "https://fleetforge.local/problems/forbidden",
					Title:         "Forbidden",
					Status:        http.StatusForbidden,
					Code:          "forbidden",
					CorrelationId: responseCorrelationID(correlationID(request.Context())),
				})
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func bearerToken(value string) (string, bool) {
	if !strings.HasPrefix(value, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(value, "Bearer ")
	return token, token != "" && !strings.ContainsAny(token, " \t\r\n")
}

func writeUnauthorized(writer http.ResponseWriter, correlation string) {
	writer.Header().Set("WWW-Authenticate", "Bearer")
	writeProblem(writer, Problem{
		Type:          "https://fleetforge.local/problems/unauthorized",
		Title:         "Unauthorized",
		Status:        http.StatusUnauthorized,
		Code:          "unauthorized",
		CorrelationId: responseCorrelationID(correlation),
	})
}

func responseCorrelationID(value string) uuid.UUID {
	correlationID, err := uuid.Parse(value)
	if err != nil {
		return uuid.New()
	}
	return correlationID
}
