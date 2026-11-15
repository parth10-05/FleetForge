// Package localartifactissuer implements the artifact application's exact
// local direct-upload capability port. Signing keys remain process-local.
package localartifactissuer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

type Issuer struct {
	secret []byte
	random io.Reader
}

func New(secret []byte, randomSource io.Reader) *Issuer {
	if randomSource == nil {
		randomSource = rand.Reader
	}
	return &Issuer{secret: append([]byte(nil), secret...), random: randomSource}
}

func (issuer *Issuer) IssueUploadCapability(_ context.Context, request app.UploadCapabilityRequest) (app.UploadCapability, error) {
	if issuer == nil || len(issuer.secret) < 32 || request.ArtifactID == "" || request.GrantID == "" || request.Container == "" || request.Key == "" || request.ExpiresAt.IsZero() {
		return app.UploadCapability{}, fmt.Errorf("invalid local upload capability request")
	}
	versionID := request.VersionID
	if versionID == "" {
		var err error
		versionID, err = issuer.newVersionID()
		if err != nil {
			return app.UploadCapability{}, fmt.Errorf("allocate local object version: %w", err)
		}
	}
	claims := localartifactstore.UploadGrantClaims{Version: "fleetforge.upload-grant/v1", GrantID: request.GrantID, ArtifactID: request.ArtifactID, Method: "PUT", Provider: artifact.ObjectProviderLocalV1, Container: request.Container, Key: request.Key, VersionID: versionID, Size: request.Metadata.Size(), Digest: request.Metadata.Digest().String(), MediaType: request.Metadata.MediaType(), ExpiresAt: request.ExpiresAt.UTC()}
	grant, err := localartifactstore.IssueSignedGrant(issuer.secret, claims)
	if err != nil {
		return app.UploadCapability{}, fmt.Errorf("sign local upload capability: %w", err)
	}
	object, err := artifact.NewObjectIdentity(claims.Provider, claims.Container, claims.Key, claims.VersionID, claims.Size, request.Metadata.Digest())
	if err != nil {
		return app.UploadCapability{}, fmt.Errorf("construct local object identity: %w", err)
	}
	return app.UploadCapability{Authorization: grant.Payload + "." + grant.Signature, Object: object, ExpiresAt: claims.ExpiresAt}, nil
}

func (issuer *Issuer) newVersionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(issuer.random, bytes); err != nil {
		return "", err
	}
	return "local-" + hex.EncodeToString(bytes), nil
}
