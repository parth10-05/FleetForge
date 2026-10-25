package controlhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

func TestRouterOwnsOnlyHumanNamespaceAndRequiresBearerCredential(t *testing.T) {
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	verifier := tokenVerifierFunc(func(_ context.Context, token string) (access.Principal, error) {
		if token != "human-token" {
			return access.Principal{}, errors.New("untrusted token")
		}
		return access.Principal{Subject: "viewer", Roles: []access.Role{access.RoleViewer}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local-synthetic-hs256"}, nil
	})
	router := NewRouter(verifier, func() time.Time { return now })

	for _, test := range []struct {
		name, path, authorization string
		want                      int
	}{
		{name: "human endpoint rejects service credential", path: "/v1/targets", authorization: "", want: http.StatusUnauthorized},
		{name: "valid bearer reaches only public router", path: "/v1/targets", authorization: "Bearer human-token", want: http.StatusNotFound},
		{name: "internal event path is not public route", path: "/v1/events/synthetic", authorization: "", want: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
