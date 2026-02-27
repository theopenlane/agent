package connectivity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
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

const (
	defaultProbePath = "/livez"
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

	targets, err := m.probeTargets()
	if err != nil {
		m.setStatus(StatusOffline, err)

		return err
	}

	client := &http.Client{
		Timeout: m.checkTimeout,
	}

	var lastErr error
	for _, target := range targets {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", target, nil)
		if reqErr != nil {
			lastErr = fmt.Errorf("failed to create request: %w", reqErr)
			continue
		}

		resp, requestErr := client.Do(req)
		if requestErr != nil {
			lastErr = requestErr
			continue
		}

		resp.Body.Close()

		if resp.StatusCode < 500 { // nolint:mnd
			m.setStatus(StatusOnline, nil)
			return nil
		}

		lastErr = fmt.Errorf("%w: %d", ErrAPIUnsuccessfulStatusCode, resp.StatusCode)
	}

	if lastErr == nil {
		lastErr = ErrAPIUnsuccessfulStatusCode
	}

	m.setStatus(StatusOffline, lastErr)

	return lastErr
}

func (m *Manager) probeTargets() ([]string, error) {
	apiURL := strings.TrimSpace(m.apiURL)
	if apiURL == "" {
		return nil, fmt.Errorf("api url is required")
	}

	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api url %q: %w", apiURL, err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid api url %q", apiURL)
	}

	// If the URL already includes an explicit endpoint path, use it as-is.
	if strings.TrimSpace(parsed.Path) != "" && parsed.Path != "/" {
		return []string{parsed.String()}, nil
	}

	baseURL := strings.TrimRight(parsed.String(), "/")
	return []string{baseURL + defaultProbePath}, nil
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
	ch, _ := m.subscribe()
	return ch
}

// SubscribeWithCancel returns a status channel and cancellation function
func (m *Manager) SubscribeWithCancel() (<-chan Status, func()) {
	ch, cancel := m.subscribe()
	return ch, cancel
}

func (m *Manager) subscribe() (chan Status, func()) {
	m.mu.Lock()

	ch := make(chan Status, 1)
	m.listeners = append(m.listeners, ch)
	currentStatus := m.status

	m.mu.Unlock()

	// Send current status immediately
	ch <- currentStatus

	return ch, func() {
		m.unsubscribe(ch)
	}
}

func (m *Manager) unsubscribe(ch chan Status) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, listener := range m.listeners {
		if listener != ch {
			continue
		}

		m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)

		close(listener)

		return
	}
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

	ch, unsubscribe := m.SubscribeWithCancel()
	defer unsubscribe()

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

// GetOutboundIP detects the preferred outbound IP address of this machine
func GetOutboundIP(ctx context.Context) (string, error) {
	// Connect to a well-known address to determine the preferred local IP
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("failed to detect outbound IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP.String(), nil
}

// GetLocalIPAddresses returns all local IP addresses (excluding loopback)
func GetLocalIPAddresses() ([]string, error) {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// Skip down interfaces and loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Skip loopback and IPv6 addresses
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}

	return ips, nil
}

// GetPreferredIPAddress returns the preferred IP address for external communication
func GetPreferredIPAddress(ctx context.Context) string {
	// Try to get the outbound IP first
	if ip, err := GetOutboundIP(ctx); err == nil {
		return ip
	}

	// Fall back to first non-loopback local IP
	if ips, err := GetLocalIPAddresses(); err == nil && len(ips) > 0 {
		// Prefer 192.168.x.x or 10.x.x.x addresses for local networks
		for _, ip := range ips {
			if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") {
				return ip
			}
		}
		// Return first available IP if no preferred patterns found
		return ips[0]
	}

	// Final fallback
	return "127.0.0.1"
}
