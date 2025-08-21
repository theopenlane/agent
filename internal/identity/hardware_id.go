package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// minHardwareIDLength is the minimum length for a hardware ID
	minHardwareIDLength = 10
	// commandTimeout is the timeout for hardware detection commands
	commandTimeout = 5 * time.Second
)

// Detector provides hardware ID detection functionality
type Detector struct {
	cachedID string
	mu       sync.RWMutex
}

// NewDetector creates a new hardware ID detector
func NewDetector() *Detector {
	return &Detector{}
}

// GetHardwareID returns the hardware ID for this system, using caching for performance
func (d *Detector) GetHardwareID() string {
	d.mu.RLock()
	if d.cachedID != "" {
		id := d.cachedID
		d.mu.RUnlock()

		return id
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check pattern
	if d.cachedID != "" {
		return d.cachedID
	}

	id := d.detectHardwareID()
	d.cachedID = id
	log.Info().Str("hardware_id", id).Str("os", runtime.GOOS).Msg("Hardware ID detected")

	return id
}

// detectHardwareID performs the actual hardware ID detection
func (d *Detector) detectHardwareID() string {
	var id string

	switch runtime.GOOS {
	case "darwin":
		id = d.detectMacOSID()
	case "linux":
		id = d.detectLinuxID()
	case "windows":
		id = d.detectWindowsID()
	default:
		log.Warn().Str("os", runtime.GOOS).Msg("Unsupported OS, using fallback ID")
	}

	// Fallback to hostname-based ID if platform detection fails
	if id == "" {
		id = d.generateFallbackID()
		log.Warn().Str("fallback_id", id).Msg("Using fallback hardware ID")
	}

	return d.normalizeID(id)
}

// detectMacOSID detects hardware ID on macOS using system_profiler
func (d *Detector) detectMacOSID() string {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "system_profiler", "SPHardwareDataType")

	output, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Msg("Failed to run system_profiler")
		return ""
	}

	outputStr := string(output)
	if idx := strings.Index(outputStr, "Hardware UUID:"); idx != -1 {
		line := outputStr[idx:]
		if endIdx := strings.Index(line, "\n"); endIdx != -1 {
			line = line[:endIdx]
		}

		parts := strings.Fields(line)
		if len(parts) >= 3 {
			uuid := parts[2]
			log.Debug().Str("uuid", uuid).Msg("Detected macOS Hardware UUID")

			return uuid
		}
	}

	log.Debug().Msg("Could not parse Hardware UUID from system_profiler output")

	return ""
}

// detectLinuxID detects hardware ID on Linux using machine-id
func (d *Detector) detectLinuxID() string {
	// Try /etc/machine-id first (systemd)
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			log.Debug().Str("source", "/etc/machine-id").Str("id", id).Msg("Detected Linux machine ID")
			return id
		}
	} else {
		log.Debug().Err(err).Msg("Failed to read /etc/machine-id")
	}

	// Try /var/lib/dbus/machine-id as fallback
	if data, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			log.Debug().Str("source", "/var/lib/dbus/machine-id").Str("id", id).Msg("Detected Linux machine ID")
			return id
		}
	} else {
		log.Debug().Err(err).Msg("Failed to read /var/lib/dbus/machine-id")
	}

	// Try DMI product UUID as last resort
	if data, err := os.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" && id != "00000000-0000-0000-0000-000000000000" {
			log.Debug().Str("source", "/sys/class/dmi/id/product_uuid").Str("id", id).Msg("Detected Linux DMI UUID")
			return id
		}
	} else {
		log.Debug().Err(err).Msg("Failed to read /sys/class/dmi/id/product_uuid")
	}

	log.Debug().Msg("Could not detect Linux machine ID")

	return ""
}

// detectWindowsID detects hardware ID on Windows using wmic
func (d *Detector) detectWindowsID() string {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wmic", "csproduct", "get", "uuid", "/value")

	output, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Msg("Failed to run wmic command")
		return ""
	}

	outputStr := string(output)
	if idx := strings.Index(outputStr, "UUID="); idx != -1 {
		uuidLine := outputStr[idx:]
		if endIdx := strings.Index(uuidLine, "\n"); endIdx != -1 {
			uuidLine = uuidLine[:endIdx]
		}

		uuid := strings.TrimSpace(strings.TrimPrefix(uuidLine, "UUID="))
		uuid = strings.TrimSpace(strings.TrimSuffix(uuid, "\r"))

		if uuid != "" && uuid != "00000000-0000-0000-0000-000000000000" {
			log.Debug().Str("uuid", uuid).Msg("Detected Windows system UUID")
			return uuid
		}
	}

	log.Debug().Msg("Could not parse UUID from wmic output")

	return ""
}

// generateFallbackID creates a fallback ID based on hostname and OS
func (d *Detector) generateFallbackID() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get hostname, using 'unknown'")

		hostname = "unknown"
	}

	// Create a deterministic ID based on hostname and OS
	hash := sha256.Sum256([]byte(hostname + runtime.GOOS + runtime.GOARCH))
	id := hex.EncodeToString(hash[:16])
	log.Debug().Str("hostname", hostname).Str("fallback_id", id).Msg("Generated fallback hardware ID")

	return id
}

// normalizeID normalizes the hardware ID to a consistent format
func (d *Detector) normalizeID(id string) string {
	if id == "" {
		return ""
	}

	// Remove any non-alphanumeric characters
	cleanID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}

		return -1
	}, id)

	// Take last N characters (or full string if shorter)
	if len(cleanID) > minHardwareIDLength {
		normalized := strings.ToUpper(cleanID[len(cleanID)-minHardwareIDLength:])
		log.Debug().Str("original", id).Str("normalized", normalized).Msg("Normalized hardware ID")

		return normalized
	}

	normalized := strings.ToUpper(cleanID)
	log.Debug().Str("original", id).Str("normalized", normalized).Msg("Normalized hardware ID (short)")

	return normalized
}

// ClearCache clears the cached hardware ID, forcing re-detection on next call
func (d *Detector) ClearCache() {
	d.mu.Lock()
	d.cachedID = ""
	d.mu.Unlock()
	log.Debug().Msg("Hardware ID cache cleared")
}

// GetCachedID returns the cached hardware ID without triggering detection
func (d *Detector) GetCachedID() string {
	d.mu.RLock()
	id := d.cachedID
	d.mu.RUnlock()

	return id
}

// ValidateID validates that a hardware ID meets the minimum requirements
func ValidateID(id string) bool {
	return len(id) >= minHardwareIDLength
}
