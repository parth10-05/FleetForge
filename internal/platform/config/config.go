// Package config parses and validates typed, role-specific process configuration.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	apiAddressKey           = "FLEETFORGE_API_ADDR"
	simulatorAddressKey     = "FLEETFORGE_SIMULATOR_ADDR"
	simulatorStatePathKey   = "FLEETFORGE_SIMULATOR_STATE_PATH"
	simulatorJournalPathKey = "FLEETFORGE_SIMULATOR_JOURNAL_PATH"
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
