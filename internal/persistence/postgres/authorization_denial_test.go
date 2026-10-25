package postgres

import "testing"

func TestAuthorizationDenialValidationAcceptsSafeFactsOnly(t *testing.T) {
	valid := AuthorizationDenial{ID: "00000000-0000-0000-0000-000000000001", ActorSubject: "admin-1", Route: "/v1/targets", Action: "register-target", ReasonCode: "permission-denied", CorrelationID: "00000000-0000-0000-0000-000000000002"}
	if !validAuthorizationDenial(valid) {
		t.Fatal("valid denial rejected")
	}
	valid.Route = "/v1/targets?token=secret"
	if validAuthorizationDenial(valid) {
		t.Fatal("unsafe route accepted")
	}
}
