package connectivity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscribeWithCancelRemovesListener(t *testing.T) {
	mgr := NewManager("https://example.com")

	ch, cancel := mgr.SubscribeWithCancel()
	if ch == nil {
		t.Fatal("expected non-nil subscription channel")
	}

	mgr.mu.RLock()
	listenerCount := len(mgr.listeners)
	mgr.mu.RUnlock()
	if listenerCount != 1 {
		t.Fatalf("expected one listener, got %d", listenerCount)
	}

	cancel()

	mgr.mu.RLock()
	listenerCount = len(mgr.listeners)
	mgr.mu.RUnlock()
	if listenerCount != 0 {
		t.Fatalf("expected no listeners after cancel, got %d", listenerCount)
	}

	// The channel may still contain the initial buffered status value.
	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting to drain initial status value")
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel after cancel")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestWaitForOnlineUnsubscribesOnContextCancel(t *testing.T) {
	mgr := NewManager("https://example.com")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.WaitForOnline(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	mgr.mu.RLock()
	listenerCount := len(mgr.listeners)
	mgr.mu.RUnlock()
	if listenerCount != 0 {
		t.Fatalf("expected no listeners after WaitForOnline returns, got %d", listenerCount)
	}
}

func TestNewManager_initialState(t *testing.T) {
	m := NewManager("https://localhost:8080")
	require.NotNil(t, m)
	assert.Equal(t, StatusUnknown, m.GetStatus())
	assert.False(t, m.IsOnline())
	assert.False(t, m.IsOffline())
}

func TestManager_setStatus_transitions(t *testing.T) {
	m := NewManager("https://localhost:8080")

	m.setStatus(StatusOnline, nil)
	assert.True(t, m.IsOnline())
	assert.False(t, m.IsOffline())

	m.setStatus(StatusOffline, ErrAPIUnsuccessfulStatusCode)
	assert.False(t, m.IsOnline())
	assert.True(t, m.IsOffline())
}

func TestManager_statusString(t *testing.T) {
	m := NewManager("")

	tests := []struct {
		status Status
		want   string
	}{
		{StatusOnline, "online"},
		{StatusOffline, "offline"},
		{StatusUnknown, "unknown"},
	}

	for _, tt := range tests {
		m.status = tt.status
		assert.Equal(t, tt.want, m.statusString())
	}
}

func TestManager_GetStats_keys(t *testing.T) {
	m := NewManager("https://localhost:8080")
	m.setStatus(StatusOnline, nil)

	stats := m.GetStats()
	require.NotNil(t, stats)
	assert.Equal(t, "online", stats["status"])
	assert.Contains(t, stats, "last_check")
}

func TestManager_WaitForOnline_alreadyOnline(t *testing.T) {
	m := NewManager("https://localhost:8080")
	m.setStatus(StatusOnline, nil)

	err := m.WaitForOnline(context.Background())
	require.NoError(t, err)
}

func TestGetPreferredIPAddress_nonEmpty(t *testing.T) {
	ip := GetPreferredIPAddress(context.Background())
	assert.NotEmpty(t, ip)
}

func TestManagerCheck_UsesLivezByDefault(t *testing.T) {
	t.Helper()

	var livezCount int
	var healthCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/livez":
			livezCount++
			w.WriteHeader(http.StatusOK)
		case "/health":
			healthCount++
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	manager := NewManager(server.URL)

	err := manager.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusOnline, manager.GetStatus())
	assert.Equal(t, 1, livezCount)
	assert.Equal(t, 0, healthCount)
}

func TestManagerCheck_UsesConfiguredPathWhenPresent(t *testing.T) {
	t.Helper()

	var configuredPathCount int
	var livezCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/custom-health":
			configuredPathCount++
			w.WriteHeader(http.StatusOK)
		case "/livez":
			livezCount++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	manager := NewManager(server.URL + "/custom-health")

	err := manager.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusOnline, manager.GetStatus())
	assert.Equal(t, 1, configuredPathCount)
	assert.Equal(t, 0, livezCount)
}
