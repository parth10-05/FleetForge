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

func signedToken(t *testing.T, key []byte, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	left := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(left))
	return left + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
