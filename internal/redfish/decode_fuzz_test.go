package redfish

import (
	"bytes"
	"testing"
)

func FuzzDecodeDocumentRejectsUnsafeInputWithoutPanic(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"Id":"one","Id":"two"}`),
		[]byte(`{"Status":{"State":"Enabled"}}`),
		[]byte(`{"broken":`),
		[]byte(`[]`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 2*DefaultDocumentMaxBytes {
			return
		}
		_, _ = DecodeDocument(bytes.NewReader(raw), DefaultDocumentMaxBytes, DefaultDocumentMaxDepth)
	})
}
