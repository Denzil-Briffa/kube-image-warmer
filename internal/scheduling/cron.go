package scheduling

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type Schedule interface {
	Next(time.Time) time.Time
}

// Parse validates a five-field Kubernetes-style cron expression.
func Parse(expression string) (Schedule, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, errors.New("schedule is required")
	}

	fieldCount := len(strings.Fields(expression))
	if fieldCount != 5 {
		return nil, fmt.Errorf(
			"schedule must contain exactly five fields, got %d",
			fieldCount,
		)
	}

	parser := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
	)

	schedule, err := parser.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf(
			"parse schedule %q: %w",
			expression,
			err,
		)
	}

	return schedule, nil
}
