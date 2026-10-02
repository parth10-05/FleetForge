// Package config parses and validates typed, role-specific process configuration.
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
)

const (
	apiAddressKey                     = "FLEETFORGE_API_ADDR"
	simulatorAddressKey               = "FLEETFORGE_SIMULATOR_ADDR"
	simulatorRunRootKey               = "FLEETFORGE_SIMULATOR_RUN_ROOT"
	simulatorScenarioPathKey          = "FLEETFORGE_SIMULATOR_SCENARIO_PATH"
	simulatorUsernameKey              = "FLEETFORGE_SIMULATOR_USERNAME"
	simulatorPasswordKey              = "FLEETFORGE_SIMULATOR_PASSWORD"
	simulatorAdminAddressKey          = "FLEETFORGE_SIMULATOR_ADMIN_ADDR"
	simulatorAdminSecretKey           = "FLEETFORGE_SIMULATOR_ADMIN_SECRET"
	objectStoreAddressKey             = "FLEETFORGE_OBJECT_STORE_ADDR"
	objectStoreRootKey                = "FLEETFORGE_OBJECT_STORE_ROOT"
	objectStoreHMACSecretKey          = "FLEETFORGE_OBJECT_STORE_HMAC_SECRET"
	postgresApplicationDSNKey         = "FLEETFORGE_POSTGRES_APP_DSN"
	postgresMigrationDSNKey           = "FLEETFORGE_POSTGRES_MIGRATION_DSN"
	postgresApplicationDSNFallbackKey = "DATABASE_URL"
	postgresMigrationDSNFallbackKey   = "DATABASE_URL_UNPOOLED"
)

type Environment string

const (
	Development Environment = "development"
	Test        Environment = "test"
	Staging     Environment = "staging"
	Production  Environment = "production"
)

// Common is shared by all roles. SecretReference is opaque and must never be logged.
type Common struct {
	Environment      Environment
	LogLevel         string
	ShutdownTimeout  time.Duration
	ReadinessTimeout time.Duration
	ReadinessFile    string
	SecretReference  string
	DebugEndpoints   bool
}

type API struct {
	Common
	Address string
}
type Worker struct{ Common }
type ArtifactWorker struct {
	Common
	Root  string
	Owner string
}
type DiscoveryWorker struct {
	Common
	Owner        string
	AllowedCIDRs []netip.Prefix
	Credentials  map[string]syntheticcredentials.Credential
}
type Simulator struct {
	Common
	Address, RunRoot, ScenarioPath, Username, Password, AdminAddress, AdminSecret string
}
type ObjectStore struct {
	Common
	Address string
	Root    string
	HMACKey []byte
}

// PostgreSQL holds bounded, role-specific database settings. DSNs are opaque
// credentials and must never be included in an error, log, or readiness body.
type PostgreSQL struct {
	ApplicationDSN     string
	MigrationDSN       string
	MaxConns           int32
	MinConns           int32
	AcquireTimeout     time.Duration
	TransactionTimeout time.Duration
	StatementTimeout   time.Duration
	MaxConnLifetime    time.Duration
	MaxConnIdleTime    time.Duration
}

// Error identifies a key and safe validation category, never the supplied value.
type Error struct{ Key, Reason string }

func (e *Error) Error() string { return fmt.Sprintf("invalid configuration %s: %s", e.Key, e.Reason) }

func LoadAPI() (API, error) {
	common, err := loadCommon(os.LookupEnv)
	if err != nil {
		return API{}, err
	}
	address, err := requiredAddress(os.LookupEnv, apiAddressKey)
	if err != nil {
		return API{}, err
	}
	return API{Common: common, Address: address}, nil
}

func LoadWorker() (Worker, error) {
	common, err := loadCommon(os.LookupEnv)
	if err != nil {
		return Worker{}, err
	}
	return Worker{Common: common}, nil
}

// LoadArtifactWorker gives the verification worker access to the exact local
// object root but deliberately not to the upload capability signing key.
func LoadArtifactWorker() (ArtifactWorker, error) {
	common, err := loadCommon(os.LookupEnv)
	if err != nil {
		return ArtifactWorker{}, err
	}
	root, err := requiredAbsolutePath(os.LookupEnv, objectStoreRootKey)
	if err != nil {
		return ArtifactWorker{}, err
	}
	owner := optional(os.LookupEnv, "FLEETFORGE_ARTIFACT_WORKER_OWNER", "artifact-verifier")
	if len(owner) > 128 || strings.TrimSpace(owner) == "" {
		return ArtifactWorker{}, &Error{Key: "FLEETFORGE_ARTIFACT_WORKER_OWNER", Reason: "must be nonempty and at most 128 bytes"}
	}
	return ArtifactWorker{Common: common, Root: root, Owner: owner}, nil
}

// LoadDiscoveryWorker accepts only explicit synthetic credentials and network
// allowlists. It never falls back to simulator admin credentials or provider
// secret references.
func LoadDiscoveryWorker() (DiscoveryWorker, error) {
	common, err := loadCommon(os.LookupEnv)
	if err != nil {
		return DiscoveryWorker{}, err
	}
	owner := optional(os.LookupEnv, "FLEETFORGE_DISCOVERY_WORKER_OWNER", "inventory-discoverer")
	if len(owner) > 128 || strings.TrimSpace(owner) == "" {
		return DiscoveryWorker{}, &Error{Key: "FLEETFORGE_DISCOVERY_WORKER_OWNER", Reason: "must be nonempty and at most 128 bytes"}
	}
	allowedCIDRs, err := requiredCIDRs(os.LookupEnv, "FLEETFORGE_DISCOVERY_ALLOWED_CIDRS")
	if err != nil {
		return DiscoveryWorker{}, err
	}
	credentials, err := requiredSyntheticCredentials(os.LookupEnv, "FLEETFORGE_DISCOVERY_CREDENTIALS_JSON")
	if err != nil {
		return DiscoveryWorker{}, err
	}
	return DiscoveryWorker{Common: common, Owner: owner, AllowedCIDRs: allowedCIDRs, Credentials: credentials}, nil
}

func LoadSimulator() (Simulator, error) {
	common, err := loadCommon(os.LookupEnv)
	if err != nil {
		return Simulator{}, err
	}
	address, err := requiredAddress(os.LookupEnv, simulatorAddressKey)
	if err != nil {
		return Simulator{}, err
	}
	runRoot, err := requiredPath(os.LookupEnv, simulatorRunRootKey)
	if err != nil {
		return Simulator{}, err
	}
	scenarioPath, err := requiredPath(os.LookupEnv, simulatorScenarioPathKey)
	if err != nil {
		return Simulator{}, err
	}
	username := optional(os.LookupEnv, simulatorUsernameKey, "")
	password := optional(os.LookupEnv, simulatorPasswordKey, "")
	if username == "" || password == "" {
		return Simulator{}, &Error{Key: simulatorUsernameKey, Reason: "and simulator password are required"}
	}
	adminAddress, err := requiredAddress(os.LookupEnv, simulatorAdminAddressKey)
	if err != nil {
		return Simulator{}, err
	}
	adminSecret := optional(os.LookupEnv, simulatorAdminSecretKey, "")
	if adminSecret == "" {
		return Simulator{}, &Error{Key: simulatorAdminSecretKey, Reason: "is required"}
	}
	return Simulator{Common: common, Address: address, RunRoot: runRoot, ScenarioPath: scenarioPath, Username: username, Password: password, AdminAddress: adminAddress, AdminSecret: adminSecret}, nil
}

// LoadObjectStore configures the independent local data-plane listener. Its
// HMAC key is decoded only for process use and must never be logged.
func LoadObjectStore() (ObjectStore, error) {
	common, err := loadCommon(os.LookupEnv)
	if err != nil {
		return ObjectStore{}, err
	}
	address, err := requiredAddress(os.LookupEnv, objectStoreAddressKey)
	if err != nil {
		return ObjectStore{}, err
	}
	root, err := requiredAbsolutePath(os.LookupEnv, objectStoreRootKey)
	if err != nil {
		return ObjectStore{}, err
	}
	encoded := optional(os.LookupEnv, objectStoreHMACSecretKey, "")
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if encoded == "" || err != nil || len(key) < 32 {
		return ObjectStore{}, &Error{Key: objectStoreHMACSecretKey, Reason: "must be base64 and decode to at least 32 bytes"}
	}
	return ObjectStore{Common: common, Address: address, Root: root, HMACKey: key}, nil
}

// LoadPostgreSQL is opt-in: Phase 01 process startup does not call it, so
// bootstrap-only roles remain database-free until they adopt persistence.
func LoadPostgreSQL() (PostgreSQL, error) {
	applicationDSN, err := preferredPostgresDSN(os.LookupEnv, postgresApplicationDSNKey, postgresApplicationDSNFallbackKey)
	if err != nil {
		return PostgreSQL{}, err
	}
	migrationDSN, err := preferredPostgresDSN(os.LookupEnv, postgresMigrationDSNKey, postgresMigrationDSNFallbackKey)
	if err != nil {
		return PostgreSQL{}, err
	}
	maxConns, err := boundedInt32(os.LookupEnv, "FLEETFORGE_POSTGRES_MAX_CONNS", 4, 1, 50)
	if err != nil {
		return PostgreSQL{}, err
	}
	minConns, err := boundedInt32(os.LookupEnv, "FLEETFORGE_POSTGRES_MIN_CONNS", 0, 0, maxConns)
	if err != nil {
		return PostgreSQL{}, err
	}
	acquireTimeout, err := duration(os.LookupEnv, "FLEETFORGE_POSTGRES_ACQUIRE_TIMEOUT", 2*time.Second, 100*time.Millisecond, 30*time.Second)
	if err != nil {
		return PostgreSQL{}, err
	}
	transactionTimeout, err := duration(os.LookupEnv, "FLEETFORGE_POSTGRES_TRANSACTION_TIMEOUT", 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return PostgreSQL{}, err
	}
	statementTimeout, err := duration(os.LookupEnv, "FLEETFORGE_POSTGRES_STATEMENT_TIMEOUT", 5*time.Second, 100*time.Millisecond, 30*time.Second)
	if err != nil {
		return PostgreSQL{}, err
	}
	maxLifetime, err := duration(os.LookupEnv, "FLEETFORGE_POSTGRES_MAX_CONN_LIFETIME", 30*time.Minute, time.Minute, time.Hour)
	if err != nil {
		return PostgreSQL{}, err
	}
	maxIdle, err := duration(os.LookupEnv, "FLEETFORGE_POSTGRES_MAX_CONN_IDLE", 5*time.Minute, time.Second, 30*time.Minute)
	if err != nil {
		return PostgreSQL{}, err
	}
	return PostgreSQL{
		ApplicationDSN: applicationDSN, MigrationDSN: migrationDSN,
		MaxConns: maxConns, MinConns: minConns,
		AcquireTimeout: acquireTimeout, TransactionTimeout: transactionTimeout,
		StatementTimeout: statementTimeout, MaxConnLifetime: maxLifetime,
		MaxConnIdleTime: maxIdle,
	}, nil
}

type lookupEnv func(string) (string, bool)

func loadCommon(lookup lookupEnv) (Common, error) {
	environment := Environment(optional(lookup, "FLEETFORGE_ENV", string(Development)))
	switch environment {
	case Development, Test, Staging, Production:
	default:
		return Common{}, &Error{Key: "FLEETFORGE_ENV", Reason: "must be development, test, staging, or production"}
	}
	logLevel := strings.ToLower(optional(lookup, "FLEETFORGE_LOG_LEVEL", "info"))
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return Common{}, &Error{Key: "FLEETFORGE_LOG_LEVEL", Reason: "must be debug, info, warn, or error"}
	}
	shutdownTimeout, err := duration(lookup, "FLEETFORGE_SHUTDOWN_TIMEOUT", 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return Common{}, err
	}
	readinessTimeout, err := duration(lookup, "FLEETFORGE_READINESS_TIMEOUT", 500*time.Millisecond, 100*time.Millisecond, 5*time.Second)
	if err != nil {
		return Common{}, err
	}
	debugEndpoints, err := boolean(lookup, "FLEETFORGE_DEBUG_ENDPOINTS", false)
	if err != nil {
		return Common{}, err
	}
	if environment == Production && debugEndpoints {
		return Common{}, &Error{Key: "FLEETFORGE_DEBUG_ENDPOINTS", Reason: "is forbidden in production"}
	}
	allowInsecure, err := boolean(lookup, "FLEETFORGE_ALLOW_INSECURE", false)
	if err != nil {
		return Common{}, err
	}
	if allowInsecure {
		return Common{}, &Error{Key: "FLEETFORGE_ALLOW_INSECURE", Reason: "is unsupported"}
	}
	return Common{Environment: environment, LogLevel: logLevel, ShutdownTimeout: shutdownTimeout, ReadinessTimeout: readinessTimeout, ReadinessFile: optional(lookup, "FLEETFORGE_READINESS_FILE", ""), SecretReference: optional(lookup, "FLEETFORGE_SECRET_REFERENCE", ""), DebugEndpoints: debugEndpoints}, nil
}

func requiredAddress(lookup lookupEnv, key string) (string, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", &Error{Key: key, Reason: "is required"}
	}
	if err := validateAddress(value); err != nil {
		return "", &Error{Key: key, Reason: "must be a host and port"}
	}
	return value, nil
}
func requiredPath(lookup lookupEnv, key string) (string, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", &Error{Key: key, Reason: "is required"}
	}
	return value, nil
}
func requiredAbsolutePath(lookup lookupEnv, key string) (string, error) {
	value, err := requiredPath(lookup, key)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(value) {
		return "", &Error{Key: key, Reason: "must be an absolute path"}
	}
	return filepath.Clean(value), nil
}
func requiredPostgresDSN(lookup lookupEnv, key string) (string, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", &Error{Key: key, Reason: "is required"}
	}
	return validatePostgresDSN(key, value)
}

// preferredPostgresDSN preserves FleetForge's explicit role-specific DSNs
// while accepting Neon CLI's standard pooled/direct environment pair for a
// locally linked development branch. An explicitly supplied FleetForge value
// always takes precedence and is never silently replaced.
func preferredPostgresDSN(lookup lookupEnv, preferredKey, fallbackKey string) (string, error) {
	if value, ok := lookup(preferredKey); ok && strings.TrimSpace(value) != "" {
		return validatePostgresDSN(preferredKey, value)
	}
	return requiredPostgresDSN(lookup, fallbackKey)
}

func validatePostgresDSN(key, value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return "", &Error{Key: key, Reason: "must be a PostgreSQL URL"}
	}
	return value, nil
}
func validateAddress(value string) error {
	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsed == 0 {
		return fmt.Errorf("invalid port")
	}
	return nil
}
func duration(lookup lookupEnv, key string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, &Error{Key: key, Reason: "must be within the supported duration range"}
	}
	return parsed, nil
}
func boundedInt32(lookup lookupEnv, key string, fallback, minimum, maximum int32) (int32, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < int64(minimum) || parsed > int64(maximum) {
		return 0, &Error{Key: key, Reason: "must be within the supported range"}
	}
	return int32(parsed), nil
}
func boolean(lookup lookupEnv, key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, &Error{Key: key, Reason: "must be a boolean"}
	}
	return parsed, nil
}
func optional(lookup lookupEnv, key, fallback string) string {
	if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func requiredCIDRs(lookup lookupEnv, key string) ([]netip.Prefix, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, &Error{Key: key, Reason: "is required"}
	}
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || len(parts) > 16 {
		return nil, &Error{Key: key, Reason: "must contain 1 to 16 CIDR prefixes"}
	}
	prefixes := make([]netip.Prefix, 0, len(parts))
	seen := make(map[netip.Prefix]struct{}, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil || !prefix.IsValid() {
			return nil, &Error{Key: key, Reason: "must contain valid CIDR prefixes"}
		}
		prefix = prefix.Masked()
		if _, exists := seen[prefix]; exists {
			return nil, &Error{Key: key, Reason: "must not repeat CIDR prefixes"}
		}
		seen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

type syntheticCredentialInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func requiredSyntheticCredentials(lookup lookupEnv, key string) (map[string]syntheticcredentials.Credential, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, &Error{Key: key, Reason: "is required"}
	}
	var input map[string]syntheticCredentialInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil || len(input) == 0 || len(input) > 64 {
		return nil, &Error{Key: key, Reason: "must be a bounded synthetic credential object"}
	}
	credentials := make(map[string]syntheticcredentials.Credential, len(input))
	for name, pair := range input {
		credential, err := syntheticcredentials.NewCredential(pair.Username, pair.Password)
		if err != nil {
			return nil, &Error{Key: key, Reason: "contains an invalid synthetic credential"}
		}
		credentials[name] = credential
	}
	if _, err := syntheticcredentials.New(credentials); err != nil {
		return nil, &Error{Key: key, Reason: "contains an invalid synthetic credential name"}
	}
	return credentials, nil
}
