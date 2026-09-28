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
type JournalStatus struct {
	Entries  uint64 `json:"entries"`
	LastHash string `json:"last_hash,omitempty"`
}
type JournalReader interface {
	Journal(context.Context) (JournalStatus, error)
}
type ResetController interface {
	Reset(context.Context) error
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
func (s *Service) Journal(ctx context.Context) (JournalStatus, error) {
	if s == nil || s.reader == nil {
		return JournalStatus{}, ErrUnavailable
	}
	reader, ok := s.reader.(JournalReader)
	if !ok {
		return JournalStatus{}, ErrUnavailable
	}
	return reader.Journal(ctx)
}
func (s *Service) Reset(ctx context.Context) error {
	if s == nil || s.reader == nil {
		return ErrUnavailable
	}
	controller, ok := s.reader.(ResetController)
	if !ok {
		return ErrUnavailable
	}
	return controller.Reset(ctx)
}
