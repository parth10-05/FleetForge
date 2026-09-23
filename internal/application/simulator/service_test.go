package simulator

import (
	"context"
	"errors"
	"testing"
)

type statusReader struct {
	status Status
	err    error
}

func (r statusReader) Status(context.Context) (Status, error) { return r.status, r.err }

func TestStatusDelegatesAndRejectsMissingReader(t *testing.T) {
	service := New(statusReader{status: Status{RunID: "run", Ready: true}})
	status, err := service.Status(context.Background())
	if err != nil || status.RunID != "run" || !status.Ready {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
	if _, err := New(nil).Status(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing reader error = %v", err)
	}
}
