package platform

import (
	"runtime"
	"slices"
	"strings"

	"github.com/theopenlane/agent/config"
)

// Selector handles platform-specific check configuration selection
type Selector struct {
	currentOS   string
	currentArch string
}

// NewSelector creates a new platform selector
func NewSelector() *Selector {
	return &Selector{
		currentOS:   runtime.GOOS,
		currentArch: runtime.GOARCH,
	}
}

// SelectVariant selects the best platform variant for the current system
// Following gitMDM's platform matching priority:
// 1. Exact OS/arch match
// 2. OS-only match
// 3. Unix family match (for non-Windows)
// 4. "all" platform match
func (s *Selector) SelectVariant(check *config.Check) *config.PlatformVariant {
	if len(check.PlatformVariants) == 0 {
		return nil
	}

	currentPlatform := s.currentOS + "/" + s.currentArch
	currentOSOnly := s.currentOS

	// 1. Look for exact platform match
	for _, variant := range check.PlatformVariants {
		if slices.Contains(variant.Platforms, currentPlatform) {
			return &variant
		}
	}

	// 2. Look for OS-only match
	for _, variant := range check.PlatformVariants {
		for _, platform := range variant.Platforms {
			if platform == currentOSOnly {
				return &variant
			}
			// Handle comma-separated platforms
			if strings.Contains(platform, ",") {
				platforms := strings.SplitSeq(platform, ",")
				for p := range platforms {
					if strings.TrimSpace(p) == currentOSOnly {
						return &variant
					}
				}
			}
		}
	}

	// 3. Look for Unix family match (if not Windows)
	if s.currentOS != "windows" {
		for _, variant := range check.PlatformVariants {
			if slices.Contains(variant.Platforms, "unix") {
				return &variant
			}
		}
	}

	// 4. Look for "all" platform match
	for _, variant := range check.PlatformVariants {
		if slices.Contains(variant.Platforms, "all") {
			return &variant
		}
	}

	return nil
}

// ApplyPlatformVariant applies a platform variant to a check configuration
// This modifies the check in-place with platform-specific settings
func (s *Selector) ApplyPlatformVariant(check *config.Check, variant *config.PlatformVariant) {
	if variant == nil {
		return
	}

	// Apply command overrides
	if variant.Command != "" {
		check.Command = variant.Command
	}

	if len(variant.Args) > 0 {
		check.Args = variant.Args
	}

	if variant.WorkDir != "" {
		check.WorkDir = variant.WorkDir
	}

	// Apply environment variables (append to existing)
	if len(variant.Env) > 0 {
		check.Env = append(check.Env, variant.Env...)
	}

	// Apply timeout override
	if variant.Timeout != nil {
		check.Timeout = *variant.Timeout
	}
}

// GetPlatformInfo returns current platform information
func (s *Selector) GetPlatformInfo() map[string]string {
	return map[string]string{
		"os":       s.currentOS,
		"arch":     s.currentArch,
		"platform": s.currentOS + "/" + s.currentArch,
	}
}

// IsSupported checks if a check supports the current platform
func (s *Selector) IsSupported(check *config.Check) bool {
	// If no platform variants, assume supported
	if len(check.PlatformVariants) == 0 {
		return true
	}

	// Check if any variant matches current platform
	return s.SelectVariant(check) != nil
}
