package localartifactstore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var ErrInvalidGrant = errors.New("invalid artifact upload grant")

// SignedGrant binds a single direct PUT capability to canonical request facts.
type SignedGrant struct{ Payload, Signature string }

// UploadGrantClaims are the complete, canonical authorization facts for one
// direct object PUT. No caller-supplied path or method is accepted outside it.
type UploadGrantClaims struct {
	Version    string    `json:"version"`
	GrantID    string    `json:"grant_id"`
	ArtifactID string    `json:"artifact_id"`
	Method     string    `json:"method"`
	Provider   string    `json:"provider"`
	Container  string    `json:"container"`
	Key        string    `json:"key"`
	VersionID  string    `json:"version_id"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	MediaType  string    `json:"media_type"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func IssueSignedGrant(secret []byte, claims UploadGrantClaims) (SignedGrant, error) {
	if len(secret) < 32 || !validClaims(claims) {
		return SignedGrant{}, ErrInvalidGrant
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return SignedGrant{}, ErrInvalidGrant
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return SignedGrant{Payload: base64.RawURLEncoding.EncodeToString(payload), Signature: base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}, nil
}
func (g SignedGrant) Verify(secret []byte, now time.Time) (UploadGrantClaims, error) {
	if len(secret) < 32 || now.IsZero() || g.Payload == "" || g.Signature == "" {
		return UploadGrantClaims{}, ErrInvalidGrant
	}
	p, e := base64.RawURLEncoding.DecodeString(g.Payload)
	if e != nil {
		return UploadGrantClaims{}, ErrInvalidGrant
	}
	s, e := base64.RawURLEncoding.DecodeString(g.Signature)
	if e != nil {
		return UploadGrantClaims{}, ErrInvalidGrant
	}
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write(p)
	if !hmac.Equal(s, m.Sum(nil)) {
		return UploadGrantClaims{}, ErrInvalidGrant
	}
	var claims UploadGrantClaims
	if err := json.Unmarshal(p, &claims); err != nil || !validClaims(claims) || !now.Before(claims.ExpiresAt) {
		return UploadGrantClaims{}, ErrInvalidGrant
	}
	return claims, nil
}

func validClaims(claims UploadGrantClaims) bool {
	if claims.Version != "fleetforge.upload-grant/v1" || claims.GrantID == "" || claims.ArtifactID == "" || claims.Method != "PUT" || claims.Provider == "" || claims.Container == "" || claims.Key == "" || claims.VersionID == "" || claims.Size <= 0 || claims.MediaType != "application/octet-stream" || claims.ExpiresAt.IsZero() {
		return false
	}
	_, err := value.ParseSHA256Digest(claims.Digest)
	return err == nil
}
