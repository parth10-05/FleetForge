package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/platform/localoidc"
)

func TestWriteScenarioCredentialsUsesOnlyOwnedChildFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "credentials.json")
	scenarioPath := filepath.Join("..", "..", "test", "fixtures", "simulator", "v1", "healthy.json")
	if err := writeScenarioCredentials(root, scenarioPath, output, "demo", "private-demo-secret"); err != nil {
		t.Fatal(err)
	}
	var credentials map[string]map[string]string
	raw, err := os.ReadFile(output)
	if err != nil || json.Unmarshal(raw, &credentials) != nil || len(credentials) != 6 {
		t.Fatalf("credential map err=%v entries=%d", err, len(credentials))
	}
	if err := writeScenarioCredentials(root, scenarioPath, filepath.Join(filepath.Dir(root), "outside.json"), "demo", "secret"); err == nil {
		t.Fatal("accepted credential output outside run root")
	}
}

func TestWriteScenarioRunIDUsesOnlyOwnedChildFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "simulator-run-id")
	scenarioPath := filepath.Join("..", "..", "test", "fixtures", "simulator", "v1", "healthy.json")
	if err := writeScenarioRunID(root, scenarioPath, output); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(output); err != nil || len(raw) != 65 {
		t.Fatalf("run ID err=%v bytes=%d", err, len(raw))
	}
	if err := writeScenarioRunID(root, scenarioPath, filepath.Join(filepath.Dir(root), "outside-run-id")); err == nil {
		t.Fatal("accepted simulator run output outside run root")
	}
}

func TestInitializeOwnsPrivateRunOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "run")
	if err := initialize(root, "seed-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".fleetforge-demo-owner")); err != nil {
		t.Fatal(err)
	}
	if err := initialize(root, "seed-1"); err == nil {
		t.Fatal("accepted existing demo run")
	}
}

func TestMintDemoTokenWritesOnlyPrivateChildFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	issuer := localoidc.Issuer{Issuer: "demo", Audience: "demo", Key: make([]byte, 32), Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	tokenFile := filepath.Join(root, "access.token")
	if err := mintDemoToken(root, tokenFile, issuer); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(tokenFile)
	if err != nil || len(raw) == 0 {
		t.Fatalf("token file: %v", err)
	}
	principal, err := (localoidc.Verifier{Issuer: "demo", Audience: "demo", Key: make([]byte, 32), RecentAuth: time.Minute, Now: func() time.Time { return time.Unix(2, 0).UTC() }}).VerifyPrincipal(t.Context(), string(raw))
	if err != nil || !principal.HasRole(access.RoleAdmin) {
		t.Fatalf("minted principal=%+v err=%v", principal, err)
	}
	if err := mintDemoToken(root, tokenFile, issuer); err == nil {
		t.Fatal("overwrote existing token")
	}
	if err := mintDemoToken(root, filepath.Join(filepath.Dir(root), "outside.token"), issuer); err == nil {
		t.Fatal("accepted token outside run root")
	}
}
