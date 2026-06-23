package actions

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrInvalidSpec = errors.New("invalid action spec")

func ValidateArgs(spec Spec, raw map[string]any) (ValidatedArgs, error) {
	if raw == nil {
		raw = map[string]any{}
	}

	argSpecs := make(map[string]ArgSpec, len(spec.Args))
	for _, arg := range spec.Args {
		argSpecs[arg.Name] = arg
	}

	for name := range raw {
		if _, ok := argSpecs[name]; !ok {
			return nil, fmt.Errorf("unknown argument %q for action %q", name, spec.Name)
		}
	}

	validated := make(ValidatedArgs, len(spec.Args))
	for _, arg := range spec.Args {
		value, ok := raw[arg.Name]
		if !ok {
			if arg.Required {
				return nil, fmt.Errorf("missing required argument %q for action %q", arg.Name, spec.Name)
			}
			continue
		}
		switch arg.Type {
		case ArgTypeString:
			s, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("argument %q for action %q must be string", arg.Name, spec.Name)
			}
			if err := validateStringArg(spec.Name, arg, s); err != nil {
				return nil, err
			}
			validated[arg.Name] = s
		default:
			return nil, fmt.Errorf("unsupported argument type %q in action %q", arg.Type, spec.Name)
		}
	}
	return validated, nil
}

func validateStringArg(action string, spec ArgSpec, value string) error {
	if len(value) < spec.MinLen || len(value) > spec.MaxLen {
		return fmt.Errorf("argument %q for action %q length must be between %d and %d", spec.Name, action, spec.MinLen, spec.MaxLen)
	}
	if containsDangerousToken(value) {
		// runner 不经过 shell，但参数入口仍然要拒绝明显的命令注入形态。这样即使未来
		// mgate.sh 内部实现出现薄弱点，agent 这一层也不会主动放大风险。
		return fmt.Errorf("argument %q for action %q contains unsafe characters", spec.Name, action)
	}
	if spec.Pattern != "" {
		ok, err := regexp.MatchString(spec.Pattern, value)
		if err != nil {
			return fmt.Errorf("invalid pattern for argument %q in action %q: %w", spec.Name, action, err)
		}
		if !ok {
			return fmt.Errorf("argument %q for action %q does not match required format", spec.Name, action)
		}
	}
	return nil
}

func containsDangerousToken(value string) bool {
	if strings.ContainsAny(value, "\x00\r\n`") {
		return true
	}
	for _, token := range []string{"..", "$(", ";", "&&", "||"} {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}
