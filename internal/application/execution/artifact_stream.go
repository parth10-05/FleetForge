package execution

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
)

var ErrExactArtifactMismatch = errors.New("exact artifact stream mismatch")

// OpenVerifiedExact opens only the persisted immutable tuple and returns a
// bounded stream. The final byte is accepted only when its size and digest
// agree with the tuple captured by admission.
func OpenVerifiedExact(ctx context.Context, reader ExactArtifactReader, object ObjectIdentity) (io.ReadCloser, error) {
	if reader == nil || ctx == nil || object.Provider == "" || object.Container == "" || object.ObjectKey == "" || object.ObjectVersion == "" || object.Size <= 0 || object.Size > 16777216 || !canonicalDigest(object.SHA256) {
		return nil, fmt.Errorf("invalid exact artifact request")
	}
	stream, err := reader.OpenExact(ctx, object)
	if err != nil {
		return nil, err
	}
	return &verifiedArtifactStream{ctx: ctx, source: stream, wantSize: object.Size, wantDigest: object.SHA256, hash: sha256.New()}, nil
}

type verifiedArtifactStream struct {
	ctx        context.Context
	source     io.ReadCloser
	wantSize   int64
	wantDigest string
	hash       hash.Hash
	read       int64
	finished   bool
}

func (s *verifiedArtifactStream) Read(p []byte) (int, error) {
	if s.finished {
		return 0, io.EOF
	}
	select {
	case <-s.ctx.Done():
		return 0, s.ctx.Err()
	default:
	}
	if int64(len(p)) > s.wantSize-s.read+1 {
		p = p[:s.wantSize-s.read+1]
	}
	n, err := s.source.Read(p)
	if n > 0 {
		s.read += int64(n)
		_, _ = s.hash.Write(p[:n])
		if s.read > s.wantSize {
			s.finished = true
			return n, fmt.Errorf("%w: object exceeds declared size", ErrExactArtifactMismatch)
		}
	}
	if err == io.EOF {
		s.finished = true
		if s.read != s.wantSize || "sha256:"+fmt.Sprintf("%x", s.hash.Sum(nil)) != s.wantDigest {
			return n, fmt.Errorf("%w: declared size or digest differs", ErrExactArtifactMismatch)
		}
	}
	return n, err
}
func (s *verifiedArtifactStream) Close() error { s.finished = true; return s.source.Close() }

func canonicalDigest(raw string) bool {
	if len(raw) != 71 || !strings.HasPrefix(raw, "sha256:") {
		return false
	}
	for _, c := range raw[7:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
