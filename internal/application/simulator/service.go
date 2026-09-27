// Package simulator exposes transport-neutral administrative status queries.
package simulator

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("simulator run unavailable")

type Status struct {
	RunID  string `json:"run_id"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}
type StatusReader interface {
	Status(context.Context) (Status, error)
}
type Metrics struct {
	Requests, AcceptedUpdates, TaskObservations, Faults uint64
}
type MetricsReader interface {
	Metrics(context.Context) (Metrics, error)
}

type Service struct{ reader StatusReader }

func New(reader StatusReader) *Service { return &Service{reader: reader} }
func (s *Service) Status(ctx context.Context) (Status, error) {
	if s == nil || s.reader == nil {
		return Status{}, ErrUnavailable
	}
	return s.reader.Status(ctx)
}
func (s *Service) Metrics(ctx context.Context) (Metrics, error) {
	if s == nil || s.reader == nil {
		return Metrics{}, ErrUnavailable
	}
	reader, ok := s.reader.(MetricsReader)
	if !ok {
		return Metrics{}, ErrUnavailable
	}
	return reader.Metrics(ctx)
}
