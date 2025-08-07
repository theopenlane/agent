package version

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// FullVersion returns the full version string
func FullVersion() string {
	return fmt.Sprintf("%s (built %s, %s)", Version, BuildDate, GitCommit)
}

// UserAgent returns the user agent string for API requests
func UserAgent() string {
	return fmt.Sprintf("openlane-agent/%s (%s; %s)", Version, runtime.GOOS, runtime.GOARCH)
}