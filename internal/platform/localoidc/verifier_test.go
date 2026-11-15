package localoidc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestVerifierRequiresSignedRecentAdminAccessToken(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	verifier := Verifier{Issuer: "https://issuer.example/test", Audience: "fleetforge-control", Key: key, RecentAuth: 15 * time.Minute, Now: func() time.Time { return now }}
	token := signedToken(t, key, map[string]any{"sub": "admin-1", "iss": "https://issuer.example/test", "client_id": "fleetforge-control", "token_use": "access", "exp": now.Add(time.Hour).Unix(), "auth_time": now.Add(-time.Minute).Unix(), "cognito:groups": []string{"firmware:resolve"}})
	principal, err := verifier.VerifyAccessToken(context.Background(), token)
	if err != nil || !principal.Verified || principal.Subject != "admin-1" || len(principal.Roles) != 1 {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
}

func TestVerifierMapsOnlyExactControlAPIGroups(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	verifier := Verifier{Issuer: "issuer", Audience: "aud", Key: key, RecentAuth: 15 * time.Minute, Now: func() time.Time { return now }}
	token := signedToken(t, key, map[string]any{"sub": "admin", "iss": "issuer", "client_id": "aud", "token_use": "access", "exp": now.Add(time.Hour).Unix(), "auth_time": now.Add(-time.Minute).Unix(), "cognito:groups": []string{"fleetforge-admin"}})
	principal, err := verifier.VerifyPrincipal(context.Background(), token)
	if err != nil || principal.Subject != "admin" || len(principal.Roles) != 1 || string(principal.Roles[0]) != "admin" {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
}

func TestVerifierRejectsStaleOrTamperedToken(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	verifier := Verifier{Issuer: "issuer", Audience: "aud", Key: key, RecentAuth: time.Minute, Now: func() time.Time { return now }}
	stale := signedToken(t, key, map[string]any{"sub": "admin", "iss": "issuer", "client_id": "aud", "token_use": "access", "exp": now.Add(time.Hour).Unix(), "auth_time": now.Add(-2 * time.Minute).Unix()})
	if _, err := verifier.VerifyAccessToken(context.Background(), stale); err == nil {
		t.Fatal("stale authentication accepted")
	}
	if _, err := verifier.VerifyAccessToken(context.Background(), stale+"x"); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestVerifierRejectsUnsupportedOrUnauthorizedTokenClaims(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	verifier := Verifier{Issuer: "issuer", Audience: "aud", Key: key, RecentAuth: time.Minute, Now: func() time.Time { return now }}
	valid := map[string]any{"sub": "admin", "iss": "issuer", "client_id": "aud", "token_use": "access", "exp": now.Add(time.Hour).Unix(), "auth_time": now.Add(-time.Minute).Unix()}
	cases := []struct {
		name   string
		header map[string]string
		claims map[string]any
	}{
		{name: "unsupported algorithm", header: map[string]string{"alg": "none", "typ": "JWT"}, claims: valid},
		{name: "wrong issuer", header: map[string]string{"alg": "HS256", "typ": "JWT"}, claims: withClaim(valid, "iss", "other-issuer")},
		{name: "wrong audience", header: map[string]string{"alg": "HS256", "typ": "JWT"}, claims: withClaim(valid, "client_id", "other-audience")},
		{name: "expired", header: map[string]string{"alg": "HS256", "typ": "JWT"}, claims: withClaim(valid, "exp", now.Add(-time.Second).Unix())},
		{name: "future authentication", header: map[string]string{"alg": "HS256", "typ": "JWT"}, claims: withClaim(valid, "auth_time", now.Add(time.Second).Unix())},
		{name: "future issued at", header: map[string]string{"alg": "HS256", "typ": "JWT"}, claims: withClaim(valid, "iat", now.Add(time.Second).Unix())},
		{name: "future not before", header: map[string]string{"alg": "HS256", "typ": "JWT"}, claims: withClaim(valid, "nbf", now.Add(time.Second).Unix())},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verifier.VerifyAccessToken(context.Background(), signedTokenWithHeader(t, key, test.header, test.claims)); err == nil {
				t.Fatal("unauthorized token accepted")
			}
		})
	}
}

func TestVerifierRejectsUnknownGroupsForSharedPrincipal(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	verifier := Verifier{Issuer: "issuer", Audience: "aud", Key: key, RecentAuth: 15 * time.Minute, Now: func() time.Time { return now }}
	token := signedToken(t, key, map[string]any{"sub": "admin", "iss": "issuer", "client_id": "aud", "token_use": "access", "exp": now.Add(time.Hour).Unix(), "auth_time": now.Add(-time.Minute).Unix(), "cognito:groups": []string{"fleetforge-admin", "unknown"}})
	if _, err := verifier.VerifyPrincipal(context.Background(), token); err == nil {
		t.Fatal("unknown group accepted for shared principal")
	}
}

func signedToken(t *testing.T, key []byte, claims map[string]any) string {
	t.Helper()
	return signedTokenWithHeader(t, key, map[string]string{"alg": "HS256", "typ": "JWT"}, claims)
}

func signedTokenWithHeader(t *testing.T, key []byte, values map[string]string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(values)
	payload, _ := json.Marshal(claims)
	left := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(left))
	return left + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func withClaim(claims map[string]any, key string, value any) map[string]any {
	copy := make(map[string]any, len(claims))
	for claim, existing := range claims {
		copy[claim] = existing
	}
	copy[key] = value
	return copy
}
