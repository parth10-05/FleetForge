//go:build postgresintegration

package v1demo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactissuer"
	"github.com/parth10-05/fleetforge/internal/platform/localoidc"
)

type acceptanceAPI struct {
	server *httptest.Server
	issuer localoidc.Issuer
}

func newAcceptanceAPI(t *testing.T, pool *pgxpool.Pool, uploadURL string) acceptanceAPI {
	t.Helper()
	key := []byte("01234567890123456789012345678901")
	now := time.Now
	issuer := localoidc.Issuer{Issuer: "fleetforge-v1demo", Audience: "fleetforge-v1demo", Key: key}
	verifier := localoidc.Verifier{Issuer: issuer.Issuer, Audience: issuer.Audience, Key: key, RecentAuth: 15 * time.Minute, Now: now}
	targets := control.NewTargetService(persistence.ControlTargetAdapter{Pool: pool}, now)
	artifacts := control.NewArtifactService(persistence.ControlArtifactAdapter{Pool: pool}, now).WithInitiation(persistence.ControlArtifactAdapter{Pool: pool}, localartifactissuer.New(key, nil), "artifacts")
	campaigns := control.NewCampaignService(persistence.ControlCampaignAdapter{Pool: pool}, now)
	attention := control.NewAttentionService(persistence.ControlAttentionAdapter{Pool: pool}, now)
	router := controlhttp.NewRouterWithResourcesUploadAndCampaigns(verifier, now, targets, artifacts, campaigns, []byte("abcdefghijklmnopqrstuvwxyz012345"), uploadURL, attention)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return acceptanceAPI{server: server, issuer: issuer}
}

func (api acceptanceAPI) token(t *testing.T, role access.Role) string {
	t.Helper()
	token, err := api.issuer.Mint(localoidc.MintRequest{Subject: "v1demo:" + string(role), Roles: []access.Role{role}, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func authenticatedRequest(t *testing.T, method, url, body, token string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}
