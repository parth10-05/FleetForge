package controlhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCorrelationMiddlewareRejectsInvalidAndReturnsGeneratedID(t *testing.T) {
	handler := withCorrelation(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if correlationID(request.Context()) == "" {
			t.Fatal("missing correlation ID")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/v1/targets", nil).WithContext(t.Context()))
	if invalid.Result().StatusCode != http.StatusNoContent || invalid.Result().Header.Get(correlationHeader) == "" {
		t.Fatalf("generated response=%d correlation=%q", invalid.Result().StatusCode, invalid.Result().Header.Get(correlationHeader))
	}
	badRequest := httptest.NewRequest(http.MethodGet, "/v1/targets", nil)
	badRequest.Header.Set(correlationHeader, "not-a-uuid")
	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, badRequest)
	if bad.Result().StatusCode != http.StatusBadRequest || bad.Result().Header.Get("Content-Type") != problemContentType {
		t.Fatalf("bad response=%d content-type=%q", bad.Result().StatusCode, bad.Result().Header.Get("Content-Type"))
	}
}

func TestDecodeJSONIsStrictAndBounded(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(`{"name":"one","unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	var target input
	if err := decodeJSON(request, &target); err == nil {
		t.Fatal("unknown field accepted")
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(`{"name":"one"}{"name":"two"}`))
	request.Header.Set("Content-Type", "application/json")
	if err := decodeJSON(request, &target); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	problem := Problem{Type: "https://fleetforge.local/problems/test", Title: "Test", Status: 400, Code: "test", CorrelationId: uuid.MustParse("00000000-0000-0000-0000-000000000001")}
	recorder := httptest.NewRecorder()
	writeProblem(recorder, problem)
	var decoded Problem
	if err := json.NewDecoder(recorder.Result().Body).Decode(&decoded); err != nil || decoded.Code != "test" {
		t.Fatalf("problem=%+v err=%v", decoded, err)
	}
}
