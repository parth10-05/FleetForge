package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/localoidc"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

type manifest struct {
	Seed      string    `json:"seed"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	root := flag.String("run-root", "", "private demo run root")
	seed := flag.String("seed", "", "deterministic scenario seed")
	tokenFile := flag.String("token-file", "", "private local access-token output file")
	scenarioPath := flag.String("scenario-path", "", "scenario used to derive synthetic credential names")
	credentialsFile := flag.String("credentials-file", "", "private local synthetic-credentials output file")
	runIDFile := flag.String("simulator-run-id-file", "", "private local simulator run-ID output file")
	simulatorUsername := flag.String("simulator-username", "", "synthetic simulator username")
	simulatorPassword := flag.String("simulator-password", "", "synthetic simulator password")
	flag.Parse()
	if err := initialize(*root, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "fleetforge demo setup failed")
		os.Exit(1)
	}
	if *tokenFile != "" {
		identity, err := config.LoadAPI()
		if err != nil || mintDemoToken(*root, *tokenFile, localoidc.Issuer{Issuer: identity.LocalIdentity.Issuer, Audience: identity.LocalIdentity.Audience, Key: identity.LocalIdentity.Key}) != nil {
			fmt.Fprintln(os.Stderr, "fleetforge demo setup failed")
			os.Exit(1)
		}
	}
	if *credentialsFile != "" || *scenarioPath != "" || *simulatorUsername != "" || *simulatorPassword != "" {
		if err := writeScenarioCredentials(*root, *scenarioPath, *credentialsFile, *simulatorUsername, *simulatorPassword); err != nil {
			fmt.Fprintln(os.Stderr, "fleetforge demo setup failed")
			os.Exit(1)
		}
	}
	if *runIDFile != "" {
		if err := writeScenarioRunID(*root, *scenarioPath, *runIDFile); err != nil {
			fmt.Fprintln(os.Stderr, "fleetforge demo setup failed")
			os.Exit(1)
		}
	}
}

func writeScenarioRunID(root, scenarioPath, outputFile string) error {
	if root == "" || scenarioPath == "" || outputFile == "" {
		return fmt.Errorf("invalid synthetic run output")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(outputFile)
	if err != nil || filepath.Dir(outputPath) != rootPath {
		return fmt.Errorf("invalid synthetic run output")
	}
	if _, err := os.Lstat(outputPath); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("synthetic run output already exists")
	}
	raw, err := os.ReadFile(scenarioPath)
	if err != nil {
		return fmt.Errorf("read synthetic scenario: %w", err)
	}
	fixture, err := scenario.Decode(raw)
	if err != nil {
		return fmt.Errorf("decode synthetic scenario: %w", err)
	}
	runID, err := scenario.ID(fixture, "run", 0)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(runID+"\n"), 0o600)
}

func writeScenarioCredentials(root, scenarioPath, credentialsFile, username, password string) error {
	if root == "" || scenarioPath == "" || credentialsFile == "" || username == "" || password == "" {
		return fmt.Errorf("invalid synthetic credential output")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(credentialsFile)
	if err != nil || filepath.Dir(outputPath) != rootPath {
		return fmt.Errorf("invalid synthetic credential output")
	}
	if _, err := os.Lstat(outputPath); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("synthetic credential output already exists")
	}
	raw, err := os.ReadFile(scenarioPath)
	if err != nil {
		return fmt.Errorf("read synthetic scenario: %w", err)
	}
	fixture, err := scenario.Decode(raw)
	if err != nil {
		return fmt.Errorf("decode synthetic scenario: %w", err)
	}
	type credential struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	credentials := make(map[string]credential, len(fixture.Targets))
	for _, target := range fixture.Targets {
		key, err := scenario.ID(fixture, "target", target.Index)
		if err != nil {
			return err
		}
		credentials[key] = credential{Username: username, Password: password}
	}
	encoded, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, encoded, 0o600)
}

func initialize(root, seed string) error {
	if root == "" || seed == "" || len(seed) > 128 {
		return fmt.Errorf("invalid demo arguments")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if filepath.Dir(abs) == abs {
		return fmt.Errorf("invalid demo root")
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return err
	}
	marker := filepath.Join(abs, ".fleetforge-demo-owner")
	if _, err := os.Stat(marker); err == nil {
		return fmt.Errorf("demo run already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte(hex.EncodeToString(bytes)), 0o600); err != nil {
		return err
	}
	encoded, err := json.Marshal(manifest{Seed: seed, CreatedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(abs, "manifest.json"), encoded, 0o600)
}

func mintDemoToken(root, tokenFile string, issuer localoidc.Issuer) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absToken, err := filepath.Abs(tokenFile)
	if err != nil || filepath.Dir(absToken) != absRoot {
		return fmt.Errorf("invalid token output")
	}
	if _, err := os.Lstat(absToken); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("token output already exists")
	}
	token, err := issuer.Mint(localoidc.MintRequest{Subject: "fleetforge-demo-admin", Roles: []access.Role{access.RoleAdmin}, TTL: 10 * time.Minute})
	if err != nil {
		return err
	}
	return os.WriteFile(absToken, []byte(token), 0o600)
}
