package scheduler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/config"
)

// Scheduler handles scheduling of compliance checks
type Scheduler struct {
	mu sync.RWMutex

	checks map[string]*ScheduledCheck
}

// ScheduledCheck wraps a Check with scheduling information
type ScheduledCheck struct {
	// Check is the compliance check configuration
	Check *config.Check
	// NextRun is the time at which the check is next scheduled to execute
	NextRun time.Time
	// LastRun is the time the check most recently completed
	LastRun time.Time
	// RunCount is the total number of times this check has been executed
	RunCount int64
	// Running indicates whether the check is currently executing
	Running bool
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

	s.mu.Lock()
	s.checks[check.Name] = &ScheduledCheck{
		Check:   check,
		NextRun: nextRun,
	}
	s.mu.Unlock()

	log.Debug().Str("check_name", check.Name).Time("next_run", nextRun).Msg("Added check")

	return nil
}

// RemoveCheck removes a check from the scheduler
func (s *Scheduler) RemoveCheck(name string) {
	s.mu.Lock()
	delete(s.checks, name)
	s.mu.Unlock()
	log.Debug().Str("check_name", name).Msg("Removed check from scheduler")
}

// GetDueChecks returns checks that are due to run
func (s *Scheduler) GetDueChecks() []*config.Check {
	now := time.Now()

	var dueChecks []*config.Check

	s.mu.RLock()

	for _, scheduled := range s.checks {
		if !scheduled.Running && (now.After(scheduled.NextRun) || now.Equal(scheduled.NextRun)) {
			dueChecks = append(dueChecks, scheduled.Check)
		}
	}

	s.mu.RUnlock()

	return dueChecks
}

// MarkRunning marks a check as currently running
func (s *Scheduler) MarkRunning(checkName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if scheduled, exists := s.checks[checkName]; exists {
		scheduled.Running = true
	}
}

// MarkCompleted marks a check as completed and calculates the next run time
func (s *Scheduler) MarkCompleted(checkName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

	scheduled, exists := s.checks[checkName]
	if !exists {
		return time.Time{}, fmt.Errorf("%w: %s", ErrCheckNotFoundInScheduler, checkName)
	}

	return scheduled.NextRun, nil
}

// GetScheduledChecks returns all scheduled checks
func (s *Scheduler) GetScheduledChecks() map[string]*ScheduledCheck {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make(map[string]*ScheduledCheck, len(s.checks))
	for name, check := range s.checks {
		copied := *check
		snapshot[name] = &copied
	}

	return snapshot
}

// calculateNextRun calculates the next run time based on a cron expression
func (s *Scheduler) calculateNextRun(cronExpr string, from time.Time) (time.Time, error) {
	schedule, err := parseCronSchedule(cronExpr)
	if err != nil {
		return time.Time{}, err
	}

	return schedule.Next(from), nil
}

func parseCronSchedule(cronExpr string) (cron.Schedule, error) {
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 && len(fields) != 6 { // nolint:mnd
		return nil, fmt.Errorf("%w: %s (expected 5 or 6 parts)", ErrInvalidCronExpression, cronExpr)
	}

	var parser cron.Parser
	if len(fields) == 6 { //nolint:mnd
		parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	} else {
		parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	}

	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidCronExpression, cronExpr)
	}

	return schedule, nil
}

// GetStats returns scheduling statistics
func (s *Scheduler) GetStats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
