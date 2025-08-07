package api

import (
	"time"

	"github.com/theopenlane/agent/internal/config"
)

// APIResponse represents the standard API response format
type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// ReportResultsRequest represents a request to report check results
type ReportResultsRequest struct {
	AgentID   string           `json:"agent_id"`
	Results   []*config.Result `json:"results"`
	Timestamp time.Time        `json:"timestamp"`
}

// JobRunnerRegistration represents agent registration data for JobRunner entity
type JobRunnerRegistration struct {
	Name         string            `json:"name"`
	IPAddress    string            `json:"ipAddress"`
	Version      string            `json:"version,omitempty"`
	Platform     string            `json:"platform,omitempty"`
	Hostname     string            `json:"hostname,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
}

// JobRunnerToken represents authentication token for job runner
type JobRunnerToken struct {
	ID        string     `json:"id"`
	Token     string     `json:"token"`
	ExpiresAt *time.Time `json:"expiresAt"`
	LastUsed  *time.Time `json:"lastUsed"`
}

// AgentStatus represents the current status of an agent
type AgentStatus struct {
	Status          string         `json:"status"`
	LastPing        time.Time      `json:"last_ping"`
	ActiveChecks    int            `json:"active_checks"`
	TotalExecutions int64          `json:"total_executions"`
	SuccessfulRuns  int64          `json:"successful_runs"`
	FailedRuns      int64          `json:"failed_runs"`
	AverageRunTime  string         `json:"average_run_time"`
	SystemInfo      SystemInfo     `json:"system_info"`
	CheckStatuses   []CheckStatus  `json:"check_statuses,omitempty"`
	Metrics         map[string]any `json:"metrics,omitempty"`
}

// SystemInfo represents system information about the agent
type SystemInfo struct {
	OS           string  `json:"os"`
	Architecture string  `json:"architecture"`
	CPUCount     int     `json:"cpu_count"`
	MemoryGB     float64 `json:"memory_gb"`
	DiskSpaceGB  float64 `json:"disk_space_gb"`
	LoadAverage  float64 `json:"load_average,omitempty"`
}

// CheckStatus represents the status of a specific check
type CheckStatus struct {
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	LastRun      time.Time `json:"last_run"`
	NextRun      time.Time `json:"next_run"`
	RunCount     int64     `json:"run_count"`
	LastDuration string    `json:"last_duration"`
	LastExitCode int       `json:"last_exit_code"`
	FindingCount int       `json:"finding_count"`
}

// RemoteCheck represents a compliance check that can be executed remotely
type RemoteCheck struct {
	// Basic info
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Execution details
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	WorkDir     string   `json:"work_dir,omitempty"`
	DownloadURL string   `json:"download_url,omitempty"`
	Platform    string   `json:"platform,omitempty"`

	// Environment and configuration
	Env      []string          `json:"env,omitempty"`
	Settings map[string]string `json:"settings,omitempty"`

	// Scheduling and execution
	Schedule string `json:"schedule,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
	Enabled  bool   `json:"enabled"`

	// Compliance context
	Controls []string `json:"controls,omitempty"`
	Tags     []string `json:"tags,omitempty"`

	// Execution options
	ContinueOnError bool `json:"continue_on_error,omitempty"`

	// Reference to scheduled job if this comes from platform
	ScheduledJobID string `json:"scheduled_job_id,omitempty"`
}

// RemoteConfig represents configuration that can be managed remotely
type RemoteConfig struct {
	Version      string            `json:"version"`
	UpdatedAt    time.Time         `json:"updated_at"`
	LogLevel     string            `json:"log_level,omitempty"`
	PollInterval string            `json:"poll_interval,omitempty"`
	Checks       []RemoteCheck     `json:"checks,omitempty"`
	Environment  map[string]string `json:"environment,omitempty"`
	Features     map[string]bool   `json:"features,omitempty"`
}

// ComplianceReport represents a summary report of compliance status
type ComplianceReport struct {
	AgentID            string                    `json:"agent_id"`
	ReportDate         time.Time                 `json:"report_date"`
	PeriodStart        time.Time                 `json:"period_start"`
	PeriodEnd          time.Time                 `json:"period_end"`
	OverallStatus      string                    `json:"overall_status"`
	TotalChecks        int                       `json:"total_checks"`
	PassedChecks       int                       `json:"passed_checks"`
	FailedChecks       int                       `json:"failed_checks"`
	TotalFindings      int                       `json:"total_findings"`
	FindingsBySeverity map[string]int            `json:"findings_by_severity"`
	ControlStatus      []ControlComplianceStatus `json:"control_status"`
	CheckSummaries     []CheckSummary            `json:"check_summaries"`
}

// ControlComplianceStatus represents compliance status for a specific control
type ControlComplianceStatus struct {
	Framework   string    `json:"framework"`
	ControlID   string    `json:"control_id"`
	ControlName string    `json:"control_name"`
	Status      string    `json:"status"`
	Coverage    float64   `json:"coverage"`
	LastChecked time.Time `json:"last_checked"`
	Evidence    []string  `json:"evidence,omitempty"`
}

// CheckSummary represents a summary of a check's results
type CheckSummary struct {
	CheckName        string    `json:"check_name"`
	LastExecuted     time.Time `json:"last_executed"`
	Status           string    `json:"status"`
	FindingCount     int       `json:"finding_count"`
	CriticalCount    int       `json:"critical_count"`
	HighCount        int       `json:"high_count"`
	MediumCount      int       `json:"medium_count"`
	LowCount         int       `json:"low_count"`
	Duration         string    `json:"duration"`
	ResourcesChecked int       `json:"resources_checked"`
}

// WebhookPayload represents a webhook notification payload
type WebhookPayload struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	AgentID   string    `json:"agent_id"`
	Data      any       `json:"data"`
}

// FindingWebhookData represents webhook data for finding events
type FindingWebhookData struct {
	CheckName string         `json:"check_name"`
	Finding   config.Finding `json:"finding"`
	NewStatus string         `json:"new_status,omitempty"`
	OldStatus string         `json:"old_status,omitempty"`
}

// AgentWebhookData represents webhook data for agent events
type AgentWebhookData struct {
	AgentName string `json:"agent_name"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// HealthCheckResponse represents the response from a health check
type HealthCheckResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Version   string            `json:"version"`
	Uptime    string            `json:"uptime"`
	Checks    map[string]string `json:"checks"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string            `json:"error"`
	Code    string            `json:"code,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// PaginatedResponse represents a paginated API response
type PaginatedResponse struct {
	APIResponse
	Pagination PaginationInfo `json:"pagination"`
}

// PaginationInfo represents pagination metadata
type PaginationInfo struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	TotalPages int  `json:"total_pages"`
	TotalItems int  `json:"total_items"`
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}
