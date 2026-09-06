package value

import (
	"errors"
	"strings"
	"time"
)

const minutesPerDay = 24 * 60

var (
	ErrInvalidTimestamp         = errors.New("invalid timestamp")
	ErrInvalidTimezone          = errors.New("invalid IANA timezone")
	ErrMissingTimezoneResolver  = errors.New("timezone resolver is required")
	ErrInvalidMaintenanceWindow = errors.New("invalid maintenance window")
)

// UTCTimestamp is an explicitly supplied, normalized UTC instant. It never
// reads a system clock.
type UTCTimestamp struct {
	value time.Time
}

func NewUTCTimestamp(raw time.Time) (UTCTimestamp, error) {
	if raw.IsZero() {
		return UTCTimestamp{}, ErrInvalidTimestamp
	}
	return UTCTimestamp{value: raw.UTC()}, nil
}

func (timestamp UTCTimestamp) Time() time.Time { return timestamp.value }

func (timestamp UTCTimestamp) IsZero() bool { return timestamp.value.IsZero() }

// TimezoneResolver supplies IANA timezone resolution at the domain boundary.
// Domain values never read host timezone configuration themselves.
type TimezoneResolver func(string) (*time.Location, error)

// TimezoneName is a validated IANA timezone identifier.
type TimezoneName struct {
	value string
}

func ParseTimezoneName(raw string, resolve TimezoneResolver) (TimezoneName, error) {
	if !isTimezoneNameSyntax(raw) {
		return TimezoneName{}, ErrInvalidTimezone
	}
	if resolve == nil {
		return TimezoneName{}, ErrMissingTimezoneResolver
	}
	location, err := resolve(raw)
	if err != nil || location == nil {
		return TimezoneName{}, ErrInvalidTimezone
	}
	return TimezoneName{value: raw}, nil
}

func (timezone TimezoneName) String() string { return timezone.value }

func (timezone TimezoneName) IsZero() bool { return timezone.value == "" }

func (timezone TimezoneName) Resolve(resolve TimezoneResolver) (*time.Location, error) {
	if timezone.IsZero() || resolve == nil {
		return nil, ErrInvalidTimezone
	}
	location, err := resolve(timezone.value)
	if err != nil || location == nil {
		return nil, ErrInvalidTimezone
	}
	return location, nil
}

// MaintenanceWindow is a weekly local-time window. Boundary inclusion and
// eligibility decisions are intentionally owned by the Phase 02 policy slice.
type MaintenanceWindow struct {
	timezone    TimezoneName
	weekdays    [7]bool
	startMinute uint16
	endMinute   uint16
}

func NewMaintenanceWindow(timezone TimezoneName, weekdays []time.Weekday, startMinute, endMinute int) (MaintenanceWindow, error) {
	if timezone.IsZero() || len(weekdays) == 0 || startMinute < 0 || startMinute >= minutesPerDay || endMinute < 0 || endMinute >= minutesPerDay || startMinute == endMinute {
		return MaintenanceWindow{}, ErrInvalidMaintenanceWindow
	}

	var selected [7]bool
	for _, weekday := range weekdays {
		if weekday < time.Sunday || weekday > time.Saturday || selected[weekday] {
			return MaintenanceWindow{}, ErrInvalidMaintenanceWindow
		}
		selected[weekday] = true
	}

	return MaintenanceWindow{
		timezone:    timezone,
		weekdays:    selected,
		startMinute: uint16(startMinute),
		endMinute:   uint16(endMinute),
	}, nil
}

func (window MaintenanceWindow) Timezone() TimezoneName { return window.timezone }

func (window MaintenanceWindow) StartMinute() int { return int(window.startMinute) }

func (window MaintenanceWindow) EndMinute() int { return int(window.endMinute) }

func (window MaintenanceWindow) IncludesWeekday(weekday time.Weekday) bool {
	return weekday >= time.Sunday && weekday <= time.Saturday && window.weekdays[weekday]
}

func (window MaintenanceWindow) Weekdays() []time.Weekday {
	weekdays := make([]time.Weekday, 0, len(window.weekdays))
	for weekday, included := range window.weekdays {
		if included {
			weekdays = append(weekdays, time.Weekday(weekday))
		}
	}
	return weekdays
}

func isTimezoneNameSyntax(raw string) bool {
	if len(raw) == 0 || len(raw) > 64 || strings.HasPrefix(raw, "/") || strings.HasSuffix(raw, "/") || strings.Contains(raw, "//") {
		return false
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		for _, character := range segment {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-' || character == '+' {
				continue
			}
			return false
		}
	}
	return true
}
