// Package config parses and validates typed, role-specific process configuration.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	apiAddressKey             = "FLEETFORGE_API_ADDR"
	simulatorAddressKey       = "FLEETFORGE_SIMULATOR_ADDR"
	simulatorStatePathKey     = "FLEETFORGE_SIMULATOR_STATE_PATH"
	simulatorJournalPathKey   = "FLEETFORGE_SIMULATOR_JOURNAL_PATH"
	postgresApplicationDSNKey = "FLEETFORGE_POSTGRES_APP_DSN"
	postgresMigrationDSNKey   = "FLEETFORGE_POSTGRES_MIGRATION_DSN"
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
type Simulator struct {
	Common
	Address, StatePath, JournalPath string
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

func LoadSimulator() (Simulator, error) {
	common, err := loadCommon(os.LookupEnv)
	if err != nil {
		return Simulator{}, err
	}
	address, err := requiredAddress(os.LookupEnv, simulatorAddressKey)
	if err != nil {
		return Simulator{}, err
	}
	statePath, err := requiredPath(os.LookupEnv, simulatorStatePathKey)
	if err != nil {
		return Simulator{}, err
	}
	journalPath, err := requiredPath(os.LookupEnv, simulatorJournalPathKey)
	if err != nil {
		return Simulator{}, err
	}
	if filepath.Clean(statePath) == filepath.Clean(journalPath) {
		return Simulator{}, &Error{Key: simulatorJournalPathKey, Reason: "must differ from simulator state path"}
	}
	return Simulator{Common: common, Address: address, StatePath: statePath, JournalPath: journalPath}, nil
}

// LoadPostgreSQL is opt-in: Phase 01 process startup does not call it, so
// bootstrap-only roles remain database-free until they adopt persistence.
func LoadPostgreSQL() (PostgreSQL, error) {
	applicationDSN, err := requiredPostgresDSN(os.LookupEnv, postgresApplicationDSNKey)
	if err != nil {
		return PostgreSQL{}, err
	}
	migrationDSN, err := requiredPostgresDSN(os.LookupEnv, postgresMigrationDSNKey)
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
func requiredPostgresDSN(lookup lookupEnv, key string) (string, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", &Error{Key: key, Reason: "is required"}
	}
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
