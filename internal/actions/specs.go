package actions

import "time"

type ArgType string

const (
	ArgTypeString ArgType = "string"
)

type ArgSpec struct {
	Name      string
	Type      ArgType
	Required  bool
	MinLen    int
	MaxLen    int
	Pattern   string
	Sensitive bool
}

type Spec struct {
	Name          string
	Args          []ArgSpec
	Timeout       time.Duration
	LongRunning   bool
	SensitiveArgs map[string]struct{}
	BuildArgv     func(ValidatedArgs) []string
}

type ValidatedArgs map[string]string

func DefaultSpecs() []Spec {
	return []Spec{
		{
			Name:    "status.snapshot",
			Timeout: 10 * time.Second,
			BuildArgv: func(ValidatedArgs) []string {
				return []string{"status", "--json"}
			},
		},
		{
			Name:    "gateway.status",
			Timeout: 10 * time.Second,
			BuildArgv: func(ValidatedArgs) []string {
				return []string{"gateway", "status", "--json"}
			},
		},
		{
			Name: "gateway.start",
			Args: []ArgSpec{
				{Name: "country", Type: ArgTypeString, Required: true, MinLen: 2, MaxLen: 2, Pattern: `^[A-Z]{2}$`},
			},
			Timeout:     60 * time.Second,
			LongRunning: true,
			BuildArgv: func(args ValidatedArgs) []string {
				return []string{"gateway", "start", "--country", args["country"]}
			},
		},
		{
			Name:        "gateway.stop",
			Timeout:     60 * time.Second,
			LongRunning: true,
			BuildArgv: func(ValidatedArgs) []string {
				return []string{"gateway", "stop"}
			},
		},
		{
			Name:    "wlan.scan",
			Timeout: 30 * time.Second,
			BuildArgv: func(ValidatedArgs) []string {
				return []string{"wlan", "scan", "--json"}
			},
		},
		{
			Name: "wlan.switch.safe",
			Args: []ArgSpec{
				{Name: "ssid", Type: ArgTypeString, Required: true, MinLen: 1, MaxLen: 32},
				{Name: "psk", Type: ArgTypeString, Required: true, MinLen: 8, MaxLen: 64, Sensitive: true},
			},
			Timeout:       180 * time.Second,
			LongRunning:   true,
			SensitiveArgs: map[string]struct{}{"psk": {}},
			BuildArgv: func(args ValidatedArgs) []string {
				return []string{"wlan", "switch-safe", "--ssid", args["ssid"], "--psk", args["psk"], "--json"}
			},
		},
	}
}

func (s Spec) Argv(args ValidatedArgs) ([]string, error) {
	if s.BuildArgv == nil {
		return nil, ErrInvalidSpec
	}
	return s.BuildArgv(args), nil
}

func (s Spec) RedactedArgs(args ValidatedArgs) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		if _, ok := s.SensitiveArgs[k]; ok {
			out[k] = "***REDACTED***"
			continue
		}
		out[k] = v
	}
	for _, arg := range s.Args {
		if arg.Sensitive {
			if _, ok := out[arg.Name]; ok {
				out[arg.Name] = "***REDACTED***"
			}
		}
	}
	return out
}
