// Package localoidc verifies test-issuer Cognito-compatible access tokens.
// It is deliberately explicit about every accepted claim; it never treats a
// decoded JWT as authenticated merely because it has administrator-looking
// fields.
package localoidc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
)

type Verifier struct {
	Issuer, Audience string
	Key              []byte
	RecentAuth       time.Duration
	Now              func() time.Time
}

func (verifier Verifier) VerifyAccessToken(ctx context.Context, token string) (execution.VerifiedPrincipal, error) {
	if err := ctx.Err(); err != nil {
		return execution.VerifiedPrincipal{}, err
	}
	if verifier.Issuer == "" || verifier.Audience == "" || len(verifier.Key) < 32 || verifier.RecentAuth <= 0 || token == "" {
		return execution.VerifiedPrincipal{}, fmt.Errorf("invalid OIDC verifier configuration")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return execution.VerifiedPrincipal{}, fmt.Errorf("malformed access token")
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := decode(parts[0], &header); err != nil || header.Algorithm != "HS256" || (header.Type != "" && header.Type != "JWT") {
		return execution.VerifiedPrincipal{}, fmt.Errorf("unsupported access token header")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return execution.VerifiedPrincipal{}, fmt.Errorf("malformed access token signature")
	}
	mac := hmac.New(sha256.New, verifier.Key)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return execution.VerifiedPrincipal{}, fmt.Errorf("invalid access token signature")
	}
	var claims struct {
		Subject   string   `json:"sub"`
		Issuer    string   `json:"iss"`
		Audience  any      `json:"aud"`
		ClientID  string   `json:"client_id"`
		TokenUse  string   `json:"token_use"`
		ExpiresAt int64    `json:"exp"`
		AuthTime  int64    `json:"auth_time"`
		Groups    []string `json:"cognito:groups"`
	}
	if err := decode(parts[1], &claims); err != nil {
		return execution.VerifiedPrincipal{}, fmt.Errorf("malformed access token claims")
	}
	now := time.Now().UTC()
	if verifier.Now != nil {
		now = verifier.Now().UTC()
	}
	authenticated := time.Unix(claims.AuthTime, 0).UTC()
	expires := time.Unix(claims.ExpiresAt, 0).UTC()
	if claims.Subject == "" || claims.Issuer != verifier.Issuer || claims.TokenUse != "access" || !matchesAudience(claims.Audience, claims.ClientID, verifier.Audience) || claims.ExpiresAt <= 0 || !expires.After(now) || claims.AuthTime <= 0 || authenticated.After(now) || now.Sub(authenticated) > verifier.RecentAuth {
		return execution.VerifiedPrincipal{}, fmt.Errorf("access token claims are not authorized")
	}
	roles := normalizeRoles(claims.Groups)
	return execution.VerifiedPrincipal{Subject: claims.Subject, IssuerClass: "local-cognito-compatible", Roles: roles, AuthenticatedAt: authenticated, ExpiresAt: expires, Verified: true}, nil
}

func decode(raw string, destination any) error {
	bytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, destination)
}
func matchesAudience(raw any, clientID, want string) bool {
	if clientID == want {
		return true
	}
	switch value := raw.(type) {
	case string:
		return value == want
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok && text == want {
				return true
			}
		}
	}
	return false
}
func normalizeRoles(groups []string) []string {
	seen := map[string]struct{}{}
	roles := make([]string, 0, len(groups))
	for _, group := range groups {
		if group == "" || len(group) > 128 || strings.ContainsAny(group, "\r\n\x00") {
			continue
		}
		if _, exists := seen[group]; !exists {
			seen[group] = struct{}{}
			roles = append(roles, group)
		}
	}
	return roles
}

var _ execution.TokenVerifier = Verifier{}
