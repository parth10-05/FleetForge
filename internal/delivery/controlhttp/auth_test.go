package controlhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

type tokenVerifierFunc func(context.Context, string) (access.Principal, error)

func (fn tokenVerifierFunc) VerifyPrincipal(ctx context.Context, token string) (access.Principal, error) {
	return fn(ctx, token)
}

func TestAuthenticationDoesNotExposeVerifierFailure(t *testing.T) {
	verifier := tokenVerifierFunc(func(context.Context, string) (access.Principal, error) {
		return access.Principal{}, errors.New("signature validation failed")
	})
	handler := withCorrelation(withAuthentication(verifier)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called for rejected token")
	})))
	request := httptest.NewRequest(http.MethodGet, "/v1/targets", nil)
	request.Header.Set("Authorization", "Bearer opaque-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || recorder.Body.String() == "" || strings.Contains(recorder.Body.String(), "signature") {
		t.Fatalf("response=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("authenticate header=%q", recorder.Header().Get("WWW-Authenticate"))
	}
}

func TestPermissionMiddlewareEnforcesRoleLattice(t *testing.T) {
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	principal := access.Principal{Subject: "viewer", Roles: []access.Role{access.RoleViewer}, AuthenticatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), IssuerClass: "local_synthetic"}
	verifier := tokenVerifierFunc(func(context.Context, string) (access.Principal, error) { return principal, nil })
	handler := withCorrelation(withAuthentication(verifier)(requirePermission(access.PermissionCreateCampaigns, func() time.Time { return now })(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("viewer granted campaign creation")
	}))))
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("response=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBearerTokenRequiresExactlyOneNonEmptyValue(t *testing.T) {
	for _, value := range []string{"", "bearer token", "Bearer", "Bearer ", "Bearer token more", "Bearer token"} {
		_, ok := bearerToken(value)
		want := value == "Bearer token"
		if ok != want {
			t.Fatalf("value=%q accepted=%t", value, ok)
		}
	}
}
