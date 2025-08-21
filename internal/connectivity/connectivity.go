package connectivity

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Status represents the connectivity status
type Status int

const (
	// StatusUnknown represents unknown connectivity status
	StatusUnknown Status = iota
	// StatusOnline represents online connectivity status
	StatusOnline
	// StatusOffline represents offline connectivity status
	StatusOffline
)

// Manager manages connectivity checking and status
type Manager struct {
	apiURL       string
	checkTimeout time.Duration

	mu            sync.RWMutex
	status        Status
	lastCheck     time.Time
	lastError     error
	lastOnlineAt  time.Time
	lastOfflineAt time.Time

	listeners []chan Status
}

// NewManager creates a new connectivity manager
func NewManager(apiURL string) *Manager {
	return &Manager{
		apiURL:       apiURL,
		checkTimeout: 10 * time.Second, // nolint:mnd
		status:       StatusUnknown,
	}
}

// Check performs a connectivity check to the API
func (m *Manager) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, m.checkTimeout)
	defer cancel()

	// Create a simple health check request
	req, err := http.NewRequestWithContext(ctx, "GET", m.apiURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{
		Timeout: m.checkTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		m.setStatus(StatusOffline, err)
		return err
	}
	defer resp.Body.Close()

	// Consider any successful response as online
	if resp.StatusCode < 500 { // nolint:mnd
		m.setStatus(StatusOnline, nil)
		return nil
	}

	err = fmt.Errorf("%w: %d", ErrAPIUnsuccessfulStatusCode, resp.StatusCode)
	m.setStatus(StatusOffline, err)

	return err
}

// GetStatus returns the current connectivity status
func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.status
}

// IsOnline returns true if currently online
func (m *Manager) IsOnline() bool {
	return m.GetStatus() == StatusOnline
}

// IsOffline returns true if currently offline
func (m *Manager) IsOffline() bool {
	return m.GetStatus() == StatusOffline
}

// Subscribe returns a channel that receives status updates
func (m *Manager) Subscribe() <-chan Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan Status, 1)
	m.listeners = append(m.listeners, ch)

	// Send current status immediately
	ch <- m.status

	return ch
}

// StartMonitoring starts periodic connectivity checks
func (m *Manager) StartMonitoring(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial check
	if err := m.Check(ctx); err != nil {
		log.Warn().Err(err).Msg("Initial connectivity check failed")
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Check(ctx); err != nil {
				log.Debug().Err(err).Msg("Connectivity check failed")
			}
		}
	}
}

// GetStats returns connectivity statistics
func (m *Manager) GetStats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]any{
		"status":     m.statusString(),
		"last_check": m.lastCheck,
	}

	if m.lastError != nil {
		stats["last_error"] = m.lastError.Error()
	}

	if !m.lastOnlineAt.IsZero() {
		stats["last_online_at"] = m.lastOnlineAt
	}

	if !m.lastOfflineAt.IsZero() {
		stats["last_offline_at"] = m.lastOfflineAt
	}

	return stats
}

// setStatus updates the connectivity status
func (m *Manager) setStatus(status Status, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldStatus := m.status
	m.status = status
	m.lastCheck = time.Now()
	m.lastError = err

	switch status {
	case StatusOnline:
		m.lastOnlineAt = time.Now()

		if oldStatus != StatusOnline {
			log.Info().Msg("Connectivity restored")
		}
	case StatusOffline:
		m.lastOfflineAt = time.Now()

		if oldStatus != StatusOffline {
			log.Warn().Err(err).Msg("Connectivity lost")
		}
	}

	// Notify listeners if status changed
	if oldStatus != status {
		for _, ch := range m.listeners {
			select {
			case ch <- status:
			default:
				// Don't block if listener is not ready
			}
		}
	}
}

// statusString returns a string representation of the status
func (m *Manager) statusString() string {
	switch m.status {
	case StatusOnline:
		return "online"
	case StatusOffline:
		return "offline"
	default:
		return "unknown"
	}
}

// WaitForOnline blocks until connectivity is restored or context is cancelled
func (m *Manager) WaitForOnline(ctx context.Context) error {
	if m.IsOnline() {
		return nil
	}

	ch := m.Subscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case status := <-ch:
			if status == StatusOnline {
				return nil
			}
		}
	}
}
