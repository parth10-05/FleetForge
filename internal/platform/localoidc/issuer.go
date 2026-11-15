package localoidc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

// Issuer mints local synthetic HS256 access tokens for tests and the
// repository-owned demo only. It is not a hosted identity provider.
type Issuer struct {
	Issuer, Audience string
	Key              []byte
	Now              func() time.Time
}

type MintRequest struct {
	Subject string
	Roles   []access.Role
	TTL     time.Duration
}

func (issuer Issuer) Mint(request MintRequest) (string, error) {
	if issuer.Issuer == "" || issuer.Audience == "" || len(issuer.Key) < 32 || request.Subject == "" || len(request.Subject) > 256 || request.TTL <= 0 || request.TTL > time.Hour {
		return "", fmt.Errorf("invalid local token mint request")
	}
	groups, err := groupsForRoles(request.Roles)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if issuer.Now != nil {
		now = issuer.Now().UTC()
	}
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", fmt.Errorf("encode local token header: %w", err)
	}
	claims, err := json.Marshal(map[string]any{
		"sub":            request.Subject,
		"iss":            issuer.Issuer,
		"client_id":      issuer.Audience,
		"token_use":      "access",
		"exp":            now.Add(request.TTL).Unix(),
		"auth_time":      now.Unix(),
		"cognito:groups": groups,
	})
	if err != nil {
		return "", fmt.Errorf("encode local token claims: %w", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, issuer.Key)
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func groupsForRoles(roles []access.Role) ([]string, error) {
	if len(roles) == 0 || len(roles) > 3 {
		return nil, fmt.Errorf("local token needs one to three roles")
	}
	groups := make([]string, 0, len(roles))
	seen := make(map[access.Role]struct{}, len(roles))
	for _, role := range roles {
		if _, exists := seen[role]; exists {
			return nil, fmt.Errorf("duplicate local token role")
		}
		seen[role] = struct{}{}
		switch role {
		case access.RoleViewer:
			groups = append(groups, "fleetforge-viewer")
		case access.RoleOperator:
			groups = append(groups, "fleetforge-operator")
		case access.RoleAdmin:
			groups = append(groups, "fleetforge-admin")
		default:
			return nil, fmt.Errorf("unsupported local token role %q", strings.TrimSpace(string(role)))
		}
	}
	return groups, nil
}
