package api

import (
	"time"

	"github.com/theopenlane/agent/config"
)

// Response represents the standard API response format
type Response struct {
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
	HardwareID   string            `json:"hardwareId,omitempty"`
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
	Version         string         `json:"version"`
	IPAddress       string         `json:"ip_address"`
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
	Response
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
