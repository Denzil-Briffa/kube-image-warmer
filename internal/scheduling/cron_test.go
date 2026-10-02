package scheduling

import (
	"testing"
	"time"
)

func TestParseAcceptsFiveFieldSchedules(t *testing.T) {
	tests := []string{
		"0 */6 * * *",
		"*/15 8-17 * * 1-5",
	}

	for _, expression := range tests {
		t.Run(expression, func(t *testing.T) {
			schedule, err := Parse(expression)
			if err != nil {
				t.Fatalf(
					"expected schedule %q to be valid: %v",
					expression,
					err,
				)
			}

			if schedule == nil {
				t.Fatalf(
					"expected schedule %q to be returned",
					expression,
				)
			}
		})
	}
}

func TestParseRejectsInvalidSchedules(t *testing.T) {
	tests := map[string]string{
		"empty":          "",
		"four fields":    "0 0 * *",
		"six fields":     "0 0 * * * *",
		"descriptor":     "@daily",
		"invalid minute": "60 0 * * *",
		"embedded zone":  "TZ=Europe/Malta 0 0 * * *",
	}

	for name, expression := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(expression); err == nil {
				t.Fatalf(
					"expected schedule %q to be rejected",
					expression,
				)
			}
		})
	}
}

func TestScheduleUsesInputLocalTimeZone(t *testing.T) {
	schedule, err := Parse("0 9 * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}

	operatorLocation := time.FixedZone(
		"operator-local",
		2*60*60,
	)
	after := time.Date(
		2026,
		time.September,
		30,
		8,
		30,
		0,
		0,
		operatorLocation,
	)

	got := schedule.Next(after)
	want := time.Date(
		2026,
		time.September,
		30,
		9,
		0,
		0,
		0,
		operatorLocation,
	)

	if !got.Equal(want) {
		t.Errorf("expected next run %v, got %v", want, got)
	}

	if got.Location() != operatorLocation {
		t.Errorf(
			"expected location %q, got %q",
			operatorLocation,
			got.Location(),
		)
	}
}
