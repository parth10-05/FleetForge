package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/delivery/healthhttp"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func TestRoutesExposeHealthAndDedicatedUploadPath(t *testing.T) {
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	health := healthhttp.NewHandler(nil, time.Second, buildinfo.Current())
	health.MarkStarted()
	handler := routes(store, []byte("01234567890123456789012345678901"), health)

	live := httptest.NewRecorder()
	handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("health status=%d", live.Code)
	}
	upload := httptest.NewRecorder()
	handler.ServeHTTP(upload, httptest.NewRequest(http.MethodPut, "/v1/objects/upload", nil))
	if upload.Code != http.StatusForbidden {
		t.Fatalf("unsigned upload status=%d", upload.Code)
	}
}
