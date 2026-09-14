package healthhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
)

type failingProbe struct{}

func (failingProbe) Check(context.Context) error { return errors.New("private-detail") }

type reasonedFailingProbe struct{}

func (reasonedFailingProbe) Check(context.Context) error  { return errors.New("private-detail") }
func (reasonedFailingProbe) ReadinessReason(error) string { return "database_schema_incompatible" }

func TestLivenessStaysHealthyWhenRequiredProbeFails(t *testing.T) {
	handler := NewHandler(failingProbe{}, time.Second, buildinfo.Info{Version: "test"})
	handler.MarkStarted()
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, want %d", response.Code, http.StatusOK)
	}
}
func TestReadinessFailsWithSafeReasonWhenProbeFails(t *testing.T) {
	handler := NewHandler(failingProbe{}, time.Second, buildinfo.Info{Version: "test"})
	handler.MarkStarted()
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if body := response.Body.String(); !strings.Contains(body, "required_probe_unavailable") || strings.Contains(body, "private-detail") {
		t.Fatalf("readiness body = %q, want safe reason only", body)
	}
}
func TestReadinessUsesReasonedProbeWithoutLeakingFailure(t *testing.T) {
	handler := NewHandler(reasonedFailingProbe{}, time.Second, buildinfo.Info{Version: "test"})
	handler.MarkStarted()
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if body := response.Body.String(); !strings.Contains(body, "database_schema_incompatible") || strings.Contains(body, "private-detail") {
		t.Fatalf("readiness body = %q, want safe database schema reason only", body)
	}
}
func TestStartupAndVersion(t *testing.T) {
	handler := NewHandler(nil, time.Second, buildinfo.Info{Version: "v1", Commit: "abc", BuiltAt: "now", Dirty: "false"})
	starting := httptest.NewRecorder()
	handler.Routes().ServeHTTP(starting, httptest.NewRequest(http.MethodGet, "/health/startup", nil))
	if starting.Code != http.StatusServiceUnavailable {
		t.Fatalf("startup before MarkStarted status = %d, want %d", starting.Code, http.StatusServiceUnavailable)
	}
	handler.MarkStarted()
	version := httptest.NewRecorder()
	handler.Routes().ServeHTTP(version, httptest.NewRequest(http.MethodGet, "/version", nil))
	if version.Code != http.StatusOK || !strings.Contains(version.Body.String(), "abc") {
		t.Fatalf("version response = (%d, %q), want build metadata", version.Code, version.Body.String())
	}
}
