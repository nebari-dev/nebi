package limits

import (
	"fmt"
	"runtime"
	"time"
)

const (
	HTTPReadHeaderTimeout = 10 * time.Second
	HTTPReadTimeout       = 30 * time.Second
	HTTPIdleTimeout       = 120 * time.Second
	HTTPMaxHeaderBytes    = 1 << 20
)

// Limits captures resource caps for request payloads and asynchronous jobs.
type Limits struct {
	RequestBodyBytes       int64 `mapstructure:"request_body_bytes" json:"request_body_bytes"`
	ManifestBytes          int   `mapstructure:"manifest_bytes" json:"manifest_bytes"`
	LockBytes              int   `mapstructure:"lock_bytes" json:"lock_bytes"`
	MetadataBytes          int   `mapstructure:"metadata_bytes" json:"metadata_bytes"`
	MaxPackages            int   `mapstructure:"max_packages" json:"max_packages"`
	PackageStringBytes     int   `mapstructure:"package_string_bytes" json:"package_string_bytes"`
	ActiveJobsPerUser      int   `mapstructure:"active_jobs_per_user" json:"active_jobs_per_user"`
	ActiveJobsPerWorkspace int   `mapstructure:"active_jobs_per_workspace" json:"active_jobs_per_workspace"`
	ActiveJobsGlobal       int   `mapstructure:"active_jobs_global" json:"active_jobs_global"`
	JobTimeoutSeconds      int   `mapstructure:"job_timeout_seconds" json:"job_timeout_seconds"`
	JobCPUSeconds          int   `mapstructure:"job_cpu_seconds" json:"job_cpu_seconds"`
	JobMemoryBytes         int64 `mapstructure:"job_memory_bytes" json:"job_memory_bytes"`
	JobProcesses           int   `mapstructure:"job_processes" json:"job_processes"`
	JobStorageBytes        int64 `mapstructure:"job_storage_bytes" json:"job_storage_bytes"`
	JobLogBytes            int   `mapstructure:"job_log_bytes" json:"job_log_bytes"`
}

// ProcessLimits captures OS-level limits for child processes.
type ProcessLimits struct {
	CPUSeconds  int
	MemoryBytes int64
	Processes   int
	FileBytes   int64
}

// Defaults returns the documented resource caps used when config omits a value.
func Defaults() Limits {
	jobCPUSeconds, jobMemoryBytes, jobProcesses := defaultProcessLimits()
	return Limits{
		RequestBodyBytes:       20 * 1024 * 1024,
		ManifestBytes:          1 * 1024 * 1024,
		LockBytes:              16 * 1024 * 1024,
		MetadataBytes:          64 * 1024,
		MaxPackages:            128,
		PackageStringBytes:     256,
		ActiveJobsPerUser:      4,
		ActiveJobsPerWorkspace: 2,
		ActiveJobsGlobal:       100,
		JobTimeoutSeconds:      2 * 60 * 60,
		JobCPUSeconds:          jobCPUSeconds,
		JobMemoryBytes:         jobMemoryBytes,
		JobProcesses:           jobProcesses,
		JobStorageBytes:        20 * 1024 * 1024 * 1024,
		JobLogBytes:            4 * 1024 * 1024,
	}
}

func defaultProcessLimits() (int, int64, int) {
	switch runtime.GOOS {
	case "linux":
		return 2 * 60 * 60, 8 * 1024 * 1024 * 1024, 256
	default:
		return 0, 0, 0
	}
}

func (l Limits) Validate() error {
	if l.RequestBodyBytes < 0 {
		return fmt.Errorf("limits.request_body_bytes must be non-negative")
	}
	if l.ManifestBytes < 0 {
		return fmt.Errorf("limits.manifest_bytes must be non-negative")
	}
	if l.LockBytes < 0 {
		return fmt.Errorf("limits.lock_bytes must be non-negative")
	}
	if l.MetadataBytes < 0 {
		return fmt.Errorf("limits.metadata_bytes must be non-negative")
	}
	if l.MaxPackages < 0 {
		return fmt.Errorf("limits.max_packages must be non-negative")
	}
	if l.PackageStringBytes < 0 {
		return fmt.Errorf("limits.package_string_bytes must be non-negative")
	}
	if l.ActiveJobsPerUser < 0 {
		return fmt.Errorf("limits.active_jobs_per_user must be non-negative")
	}
	if l.ActiveJobsPerWorkspace < 0 {
		return fmt.Errorf("limits.active_jobs_per_workspace must be non-negative")
	}
	if l.ActiveJobsGlobal < 0 {
		return fmt.Errorf("limits.active_jobs_global must be non-negative")
	}
	if l.JobTimeoutSeconds < 0 {
		return fmt.Errorf("limits.job_timeout_seconds must be non-negative")
	}
	if l.JobCPUSeconds < 0 {
		return fmt.Errorf("limits.job_cpu_seconds must be non-negative")
	}
	if l.JobMemoryBytes < 0 {
		return fmt.Errorf("limits.job_memory_bytes must be non-negative")
	}
	if l.JobProcesses < 0 {
		return fmt.Errorf("limits.job_processes must be non-negative")
	}
	if l.JobStorageBytes < 0 {
		return fmt.Errorf("limits.job_storage_bytes must be non-negative")
	}
	if l.JobLogBytes < 0 {
		return fmt.Errorf("limits.job_log_bytes must be non-negative")
	}
	return nil
}

func (l Limits) JobTimeout() time.Duration {
	if l.JobTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(l.JobTimeoutSeconds) * time.Second
}

func (l Limits) ProcessLimits() ProcessLimits {
	fileBytes := l.JobStorageBytes
	if runtime.GOOS == "windows" {
		fileBytes = 0
	}
	return ProcessLimits{
		CPUSeconds:  l.JobCPUSeconds,
		MemoryBytes: l.JobMemoryBytes,
		Processes:   l.JobProcesses,
		FileBytes:   fileBytes,
	}
}

func (l Limits) HTTPWriteTimeout() time.Duration {
	timeout := l.JobTimeout()
	if timeout <= 0 {
		return 0
	}
	return timeout + time.Minute
}
