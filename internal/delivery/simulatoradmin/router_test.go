package simulatoradmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/simulator"
	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
)

type reader struct{}

func (reader) Status(context.Context) (simulator.Status, error) {
	return simulator.Status{RunID: "synthetic-run", Ready: true}, nil
}

func TestAdminAuthAndListenerIsolation(t *testing.T) {
	admin := New(simulator.New(reader{}), []byte("admin-secret"), time.Second).Routes()
	for _, testCase := range []struct {
		name, credential string
		want             int
	}{
		{name: "missing", credential: "", want: http.StatusUnauthorized},
		{name: "wrong", credential: "wrong", want: http.StatusUnauthorized},
		{name: "correct", credential: "admin-secret", want: http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/admin/v1/status", nil)
			request.Header.Set(credentialHeader, testCase.credential)
			response := httptest.NewRecorder()
			admin.ServeHTTP(response, request)
			if response.Code != testCase.want {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, "/admin/v1/status", nil)
	response := httptest.NewRecorder()
	redfishsim.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("Redfish listener exposed admin route: %d", response.Code)
	}
}
