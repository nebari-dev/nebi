package limits

import (
	"fmt"
	"runtime"
	"time"
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
	JobStorageBytes        int64 `mapstructure:"job_storage_bytes" json:"job_storage_bytes"`
	JobLogBytes            int   `mapstructure:"job_log_bytes" json:"job_log_bytes"`
}

// ProcessLimits captures OS-level limits for child processes.
type ProcessLimits struct {
	CPUSeconds int
	FileBytes  int64
}

// Defaults returns the documented resource caps used when config omits a value.
func Defaults() Limits {
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
		JobCPUSeconds:          defaultCPUSeconds(),
		JobStorageBytes:        20 * 1024 * 1024 * 1024,
		JobLogBytes:            4 * 1024 * 1024,
	}
}

func defaultCPUSeconds() int {
	if runtime.GOOS == "windows" {
		return 0
	}
	return 2 * 60 * 60
}

func (l Limits) Validate() error {
	checks := []struct {
		name  string
		value int64
	}{
		{"request_body_bytes", l.RequestBodyBytes},
		{"manifest_bytes", int64(l.ManifestBytes)},
		{"lock_bytes", int64(l.LockBytes)},
		{"metadata_bytes", int64(l.MetadataBytes)},
		{"max_packages", int64(l.MaxPackages)},
		{"package_string_bytes", int64(l.PackageStringBytes)},
		{"active_jobs_per_user", int64(l.ActiveJobsPerUser)},
		{"active_jobs_per_workspace", int64(l.ActiveJobsPerWorkspace)},
		{"active_jobs_global", int64(l.ActiveJobsGlobal)},
		{"job_timeout_seconds", int64(l.JobTimeoutSeconds)},
		{"job_cpu_seconds", int64(l.JobCPUSeconds)},
		{"job_storage_bytes", l.JobStorageBytes},
		{"job_log_bytes", int64(l.JobLogBytes)},
	}
	for _, check := range checks {
		if check.value < 0 {
			return fmt.Errorf("limits.%s must be non-negative", check.name)
		}
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
		CPUSeconds: l.JobCPUSeconds,
		FileBytes:  fileBytes,
	}
}

func (l Limits) HTTPWriteTimeout() time.Duration {
	timeout := l.JobTimeout()
	if timeout <= 0 {
		return 0
	}
	return timeout + time.Minute
}
