package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/config"
)

// Scheduler handles scheduling of compliance checks
type Scheduler struct {
	checks map[string]*ScheduledCheck
}

// ScheduledCheck wraps a Check with scheduling information
type ScheduledCheck struct {
	Check    *config.Check
	NextRun  time.Time
	LastRun  time.Time
	RunCount int64
	Running  bool
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{
		checks: make(map[string]*ScheduledCheck),
	}
}

// AddCheck adds a check to the scheduler
func (s *Scheduler) AddCheck(check *config.Check) error {
	if !check.Enabled {
		return nil // Skip disabled checks
	}

	nextRun, err := s.calculateNextRun(check.Schedule, time.Now())
	if err != nil {
		return fmt.Errorf("failed to calculate next run for check %s: %w", check.Name, err)
	}

	s.checks[check.Name] = &ScheduledCheck{
		Check:   check,
		NextRun: nextRun,
	}

	log.Debug().Str("check_name", check.Name).Time("next_run", nextRun).Msg("Added check")

	return nil
}

// RemoveCheck removes a check from the scheduler
func (s *Scheduler) RemoveCheck(name string) {
	delete(s.checks, name)
	log.Debug().Str("check_name", name).Msg("Removed check from scheduler")
}

// GetDueChecks returns checks that are due to run
func (s *Scheduler) GetDueChecks() []*config.Check {
	now := time.Now()

	var dueChecks []*config.Check

	for _, scheduled := range s.checks {
		if !scheduled.Running && (now.After(scheduled.NextRun) || now.Equal(scheduled.NextRun)) {
			dueChecks = append(dueChecks, scheduled.Check)
		}
	}

	return dueChecks
}

// MarkRunning marks a check as currently running
func (s *Scheduler) MarkRunning(checkName string) {
	if scheduled, exists := s.checks[checkName]; exists {
		scheduled.Running = true
	}
}

// MarkCompleted marks a check as completed and calculates the next run time
func (s *Scheduler) MarkCompleted(checkName string) error {
	scheduled, exists := s.checks[checkName]
	if !exists {
		return fmt.Errorf("%w: %s", ErrCheckNotFoundInScheduler, checkName)
	}

	now := time.Now()
	scheduled.LastRun = now
	scheduled.RunCount++
	scheduled.Running = false

	// Calculate next run time
	nextRun, err := s.calculateNextRun(scheduled.Check.Schedule, now)
	if err != nil {
		return fmt.Errorf("failed to calculate next run for check %s: %w", checkName, err)
	}

	scheduled.NextRun = nextRun
	log.Debug().Str("check_name", checkName).Time("next_run", nextRun).Msg("Check completed")

	return nil
}

// GetNextRunTime returns the next run time for a specific check
func (s *Scheduler) GetNextRunTime(checkName string) (time.Time, error) {
	scheduled, exists := s.checks[checkName]
	if !exists {
		return time.Time{}, fmt.Errorf("%w: %s", ErrCheckNotFoundInScheduler, checkName)
	}

	return scheduled.NextRun, nil
}

// GetScheduledChecks returns all scheduled checks
func (s *Scheduler) GetScheduledChecks() map[string]*ScheduledCheck {
	return s.checks
}

// calculateNextRun calculates the next run time based on a cron expression
func (s *Scheduler) calculateNextRun(cronExpr string, from time.Time) (time.Time, error) {
	// This is a simplified cron parser
	// In production, you'd want to use a proper cron library like robfig/cron
	parts := strings.Fields(cronExpr)
	if len(parts) != 5 { // nolint:mnd
		return time.Time{}, fmt.Errorf("%w: %s (expected 5 parts)", ErrInvalidCronExpression, cronExpr)
	}

	// Parse: minValute hour day month weekday
	minValute, err := s.parseCronField(parts[0], 0, 59) // nolint:mnd
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid minValute field: %w", err)
	}

	hour, err := s.parseCronField(parts[1], 0, 23) // nolint:mnd
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hour field: %w", err)
	}

	// For simplicity, we'll handle basic cases:
	// */X - every X units
	// X - specific value
	// * - every unit

	next := from.Add(1 * time.Minute).Truncate(time.Minute)

	// Simple scheduling logic
	if minValute.isWildcard && hour.isWildcard {
		// Run every minValute (for testing)
		return next, nil
	}

	if minValute.isInterval {
		// Every X minValutes
		minValutesToAdd := minValute.interval - (next.Minute() % minValute.interval)
		if minValutesToAdd == minValute.interval {
			minValutesToAdd = 0
		}

		return next.Add(time.Duration(minValutesToAdd) * time.Minute), nil
	}

	if hour.isInterval && !minValute.isWildcard {
		// Every X hours at specific minValute
		targetMinute := minValute.value
		nextHour := next.Hour()

		if hour.interval > 0 {
			hoursToAdd := hour.interval - (nextHour % hour.interval)
			if hoursToAdd == hour.interval {
				hoursToAdd = 0
			}

			next = next.Add(time.Duration(hoursToAdd) * time.Hour)
		}

		// Set to target minValute
		next = time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), targetMinute, 0, 0, next.Location())

		// If we've passed the target time today, move to next occurrence
		if next.Before(from) {
			if hour.interval > 0 {
				next = next.Add(time.Duration(hour.interval) * time.Hour)
			} else {
				next = next.Add(24 * time.Hour) // nolint:mnd
			}
		}

		return next, nil
	}

	// Specific time (hour and minValute)
	if !minValute.isWildcard && !hour.isWildcard && !minValute.isInterval && !hour.isInterval {
		next = time.Date(next.Year(), next.Month(), next.Day(), hour.value, minValute.value, 0, 0, next.Location())

		// If we've passed this time today, schedule for tomorrow
		if next.Before(from) {
			next = next.Add(24 * time.Hour) // nolint:mnd
		}

		return next, nil
	}

	// Default: run in 1 minValute (fallback)
	return from.Add(1 * time.Minute), nil
}

type cronField struct {
	isWildcard bool
	isInterval bool
	value      int
	interval   int
}

// parseCronField parses a single field from a cron expression
func (s *Scheduler) parseCronField(field string, minVal, maxVal int) (cronField, error) {
	result := cronField{}

	if field == "*" {
		result.isWildcard = true
		return result, nil
	}

	if strings.HasPrefix(field, "*/") {
		// Interval: */5 means every 5 units
		intervalStr := field[2:]

		interval, err := strconv.Atoi(intervalStr)
		if err != nil {
			return result, fmt.Errorf("%w: %s", ErrInvalidInterval, field)
		}

		if interval < 1 || interval > maxVal {
			return result, fmt.Errorf("%w: %d out of range [1, %d]", ErrIntervalOutOfRange, interval, maxVal)
		}

		result.isInterval = true
		result.interval = interval

		return result, nil
	}

	// Specific value
	value, err := strconv.Atoi(field)
	if err != nil {
		return result, fmt.Errorf("%w: %s", ErrInvalidNumber, field)
	}

	if value < minVal || value > maxVal {
		return result, fmt.Errorf("%w: %d out of range [%d, %d]", ErrValueOutOfRange, value, minVal, maxVal)
	}

	result.value = value

	return result, nil
}

// GetStats returns scheduling statistics
func (s *Scheduler) GetStats() map[string]any {
	stats := map[string]any{
		"total_checks": len(s.checks),
		"checks":       make(map[string]any),
	}

	now := time.Now()
	dueCount := 0
	runningCount := 0

	for name, scheduled := range s.checks {
		if now.After(scheduled.NextRun) && !scheduled.Running {
			dueCount++
		}

		if scheduled.Running {
			runningCount++
		}

		stats["checks"].(map[string]any)[name] = map[string]any{
			"next_run":  scheduled.NextRun,
			"last_run":  scheduled.LastRun,
			"run_count": scheduled.RunCount,
			"is_due":    now.After(scheduled.NextRun) && !scheduled.Running,
			"running":   scheduled.Running,
		}
	}

	stats["due_checks"] = dueCount
	stats["running_checks"] = runningCount

	return stats
}
