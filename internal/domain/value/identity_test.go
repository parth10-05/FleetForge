package value

import (
	"errors"
	"strings"
	"testing"
)

func TestLabelKeyNormalizesCaseAndRejectsInvalidInput(t *testing.T) {
	key, err := ParseLabelKey("Failure_Domain.Site")
	if err != nil {
		t.Fatalf("ParseLabelKey() error = %v", err)
	}
	if key.String() != "failure_domain.site" {
		t.Fatalf("normalized key = %q", key.String())
	}

	for _, raw := range []string{"", "site ", "-site", "site-", "site/value", "sité", strings.Repeat("a", maxLabelKeyLength+1)} {
		_, err := ParseLabelKey(raw)
		if !errors.Is(err, ErrInvalidLabelKey) {
			t.Fatalf("ParseLabelKey(%q) error = %v, want ErrInvalidLabelKey", raw, err)
		}
	}
}

func TestLabelValueRejectsControlsInvalidUTF8AndUnboundedInput(t *testing.T) {
	for _, raw := range []string{"", "rack\n1", string([]byte{0xff}), strings.Repeat("x", maxLabelValueLength+1)} {
		_, err := ParseLabelValue(raw)
		if !errors.Is(err, ErrInvalidLabelValue) {
			t.Fatalf("ParseLabelValue(%q) error = %v, want ErrInvalidLabelValue", raw, err)
		}
	}
	value, err := ParseLabelValue("rack-α")
	if err != nil || value.String() != "rack-α" {
		t.Fatalf("valid Unicode label value = %q, %v", value.String(), err)
	}
}

func TestFailureDomainLabelsRequireSiteRackNormalizeAndCopy(t *testing.T) {
	input := map[string]string{"SITE": "iad", "rack": "r-01", "role": "control"}
	labels, err := NewFailureDomainLabels(input)
	if err != nil {
		t.Fatalf("NewFailureDomainLabels() error = %v", err)
	}
	if labels.Site().String() != "iad" || labels.Rack().String() != "r-01" {
		t.Fatalf("site/rack = %q/%q", labels.Site(), labels.Rack())
	}
	roleKey, _ := ParseLabelKey("role")
	role, exists := labels.Get(roleKey)
	if !exists || role.String() != "control" {
		t.Fatalf("extensible role label = %q, exists %t", role, exists)
	}

	input["SITE"] = "changed"
	returned := labels.AsMap()
	returned["site"] = "changed again"
	if labels.Site().String() != "iad" || labels.AsMap()["site"] != "iad" {
		t.Fatal("labels exposed mutable storage")
	}

	for _, raw := range []map[string]string{
		{"rack": "r-01"},
		{"site": "iad"},
		{"site": "iad", "SITE": "other", "rack": "r-01"},
	} {
		_, err := NewFailureDomainLabels(raw)
		if !errors.Is(err, ErrMissingRequiredLabel) && !errors.Is(err, ErrDuplicateLabelKey) {
			t.Fatalf("NewFailureDomainLabels(%v) error = %v", raw, err)
		}
	}
}
