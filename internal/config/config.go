package config

const DefaultPath = "/etc/mgate-agent/agent.yaml"

type Config struct {
	Cloud    CloudConfig    `yaml:"cloud" json:"cloud"`
	Agent    AgentConfig    `yaml:"agent" json:"agent"`
	Security SecurityConfig `yaml:"security" json:"security"`
	Logging  LoggingConfig  `yaml:"logging" json:"logging"`
}

type CloudConfig struct {
	BaseURL           string `yaml:"base_url" json:"base_url"`
	WSPath            string `yaml:"ws_path" json:"ws_path"`
	PullPath          string `yaml:"pull_path" json:"pull_path"`
	ResultPath        string `yaml:"result_path" json:"result_path"`
	StatusPath        string `yaml:"status_path" json:"status_path"`
	RequestTimeoutSec int    `yaml:"request_timeout_sec" json:"request_timeout_sec"`
	PullIntervalSec   int    `yaml:"pull_interval_sec" json:"pull_interval_sec"`
	WSEnabled         bool   `yaml:"ws_enabled" json:"ws_enabled"`
	PullEnabled       bool   `yaml:"pull_enabled" json:"pull_enabled"`
}

type AgentConfig struct {
	DeviceName               string `yaml:"device_name" json:"device_name"`
	MGatePath                string `yaml:"mgate_path" json:"mgate_path"`
	WorkDir                  string `yaml:"work_dir" json:"work_dir"`
	HeartbeatIntervalSec     int    `yaml:"heartbeat_interval_sec" json:"heartbeat_interval_sec"`
	StatusIntervalSec        int    `yaml:"status_interval_sec" json:"status_interval_sec"`
	DefaultCommandTimeoutSec int    `yaml:"default_command_timeout_sec" json:"default_command_timeout_sec"`
	LongCommandTimeoutSec    int    `yaml:"long_command_timeout_sec" json:"long_command_timeout_sec"`
	MaxParallelJobs          int    `yaml:"max_parallel_jobs" json:"max_parallel_jobs"`
	MaxOutputBytes           int    `yaml:"max_output_bytes" json:"max_output_bytes"`
}

type SecurityConfig struct {
	CredentialsFile string   `yaml:"credentials_file" json:"credentials_file"`
	AllowActions    []string `yaml:"allow_actions" json:"allow_actions"`
	ClockSkewSec    int      `yaml:"clock_skew_sec" json:"clock_skew_sec"`
	StrictWhitelist bool     `yaml:"strict_whitelist" json:"strict_whitelist"`
}

type LoggingConfig struct {
	Level     string `yaml:"level" json:"level"`
	AuditFile string `yaml:"audit_file" json:"audit_file"`
}
