package redfish

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeDocumentRejectsAmbiguousAndUnboundedInput(t *testing.T) {
	cases := []struct {
		name string
		body string
		want error
	}{
		{name: "duplicate top-level key", body: `{"Id":"one","Id":"two"}`, want: ErrMalformedDocument},
		{name: "duplicate nested key", body: `{"Status":{"State":"OK","State":"Bad"}}`, want: ErrMalformedDocument},
		{name: "trailing document", body: `{"Id":"one"} {"Id":"two"}`, want: ErrMalformedDocument},
		{name: "malformed", body: `{"Id":`, want: ErrMalformedDocument},
		{name: "oversize", body: `{"Id":"four"}`, want: ErrResponseTooLarge},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			maxBytes := DefaultDocumentMaxBytes
			if test.name == "oversize" {
				maxBytes = 4
			}
			_, err := DecodeDocument(strings.NewReader(test.body), maxBytes, DefaultDocumentMaxDepth)
			if !errors.Is(err, test.want) {
				t.Fatalf("DecodeDocument error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDecodeDocumentPreservesSingleBoundedDocument(t *testing.T) {
	body := `{"@odata.id":"/redfish/v1/Systems/1","Status":{"State":"Enabled"},"Firmware":["1.0"]}`
	got, err := DecodeDocument(strings.NewReader(body), DefaultDocumentMaxBytes, DefaultDocumentMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("wire document = %q, want %q", got, body)
	}
}

func TestDecodeDocumentRejectsExcessiveNesting(t *testing.T) {
	_, err := DecodeDocument(strings.NewReader(`{"a":{"b":{"c":1}}}`), DefaultDocumentMaxBytes, 2)
	if !errors.Is(err, ErrMalformedDocument) {
		t.Fatalf("DecodeDocument error = %v, want malformed document", err)
	}
}
