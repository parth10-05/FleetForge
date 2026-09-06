package value

import (
	"errors"
	"testing"
	"time"
)

func TestUTCTimestampNormalizesExplicitInstant(t *testing.T) {
	input := time.Date(2024, time.March, 10, 1, 30, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	timestamp, err := NewUTCTimestamp(input)
	if err != nil {
		t.Fatalf("NewUTCTimestamp() error = %v", err)
	}
	got := timestamp.Time()
	if got.Location() != time.UTC || got.Hour() != 20 || got.Day() != 9 {
		t.Fatalf("normalized timestamp = %s, want UTC 2024-03-09T20:00:00Z", got)
	}

	_, err = NewUTCTimestamp(time.Time{})
	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Fatalf("zero timestamp error = %v, want ErrInvalidTimestamp", err)
	}
}

func TestTimezoneNameUsesInjectedResolverAndPreservesDST(t *testing.T) {
	resolver := TimezoneResolver(time.LoadLocation)
	timezone, err := ParseTimezoneName("America/New_York", resolver)
	if err != nil {
		t.Fatalf("ParseTimezoneName() error = %v", err)
	}
	location, err := timezone.Resolve(resolver)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	timestamp, err := NewUTCTimestamp(time.Date(2024, time.March, 10, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewUTCTimestamp() error = %v", err)
	}
	local := timestamp.Time().In(location)
	zone, _ := local.Zone()
	if local.Hour() != 3 || zone != "EDT" {
		t.Fatalf("DST conversion = %s, want 03:00 EDT", local)
	}
}

func TestTimezoneNameRejectsInvalidNamesAndMissingResolver(t *testing.T) {
	resolver := TimezoneResolver(time.LoadLocation)
	for _, raw := range []string{"", "/America/New_York", "America//New_York", "America/../New_York", "Not/AZone", "America/New York"} {
		_, err := ParseTimezoneName(raw, resolver)
		if !errors.Is(err, ErrInvalidTimezone) {
			t.Fatalf("ParseTimezoneName(%q) error = %v, want ErrInvalidTimezone", raw, err)
		}
	}
	_, err := ParseTimezoneName("UTC", nil)
	if !errors.Is(err, ErrMissingTimezoneResolver) {
		t.Fatalf("nil resolver error = %v, want ErrMissingTimezoneResolver", err)
	}
}

func TestMaintenanceWindowCopiesWeekdaysAndRejectsAmbiguity(t *testing.T) {
	timezone, err := ParseTimezoneName("UTC", TimezoneResolver(time.LoadLocation))
	if err != nil {
		t.Fatalf("ParseTimezoneName() error = %v", err)
	}
	inputDays := []time.Weekday{time.Monday, time.Wednesday}
	window, err := NewMaintenanceWindow(timezone, inputDays, 60, 120)
	if err != nil {
		t.Fatalf("NewMaintenanceWindow() error = %v", err)
	}
	inputDays[0] = time.Friday
	if !window.IncludesWeekday(time.Monday) || window.IncludesWeekday(time.Friday) {
		t.Fatal("maintenance window retained caller weekday storage")
	}
	if window.StartMinute() != 60 || window.EndMinute() != 120 {
		t.Fatalf("window minutes = %d-%d, want 60-120", window.StartMinute(), window.EndMinute())
	}

	invalidInputs := []struct {
		weekdays []time.Weekday
		start    int
		end      int
	}{
		{weekdays: nil, start: 60, end: 120},
		{weekdays: []time.Weekday{time.Monday, time.Monday}, start: 60, end: 120},
		{weekdays: []time.Weekday{7}, start: 60, end: 120},
		{weekdays: []time.Weekday{time.Monday}, start: -1, end: 120},
		{weekdays: []time.Weekday{time.Monday}, start: 60, end: 1440},
		{weekdays: []time.Weekday{time.Monday}, start: 60, end: 60},
	}
	for _, input := range invalidInputs {
		_, err := NewMaintenanceWindow(timezone, input.weekdays, input.start, input.end)
		if !errors.Is(err, ErrInvalidMaintenanceWindow) {
			t.Fatalf("NewMaintenanceWindow(%v, %d, %d) error = %v, want ErrInvalidMaintenanceWindow", input.weekdays, input.start, input.end, err)
		}
	}
}
