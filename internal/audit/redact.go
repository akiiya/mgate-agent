package audit

import "strings"

const RedactedValue = "***REDACTED***"

func Redact(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isSensitiveKey(key) {
				out[key] = RedactedValue
				continue
			}
			out[key] = Redact(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = Redact(item)
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	if k == "psk" || k == "key" || k == "device_secret" {
		return true
	}
	return strings.Contains(k, "password") || strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.HasSuffix(k, "_key")
}

func RedactText(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.SplitAfter(s, "\n")
	for i, line := range lines {
		if containsSensitiveWord(line) {
			if strings.HasSuffix(line, "\n") {
				lines[i] = RedactedValue + "\n"
			} else {
				lines[i] = RedactedValue
			}
		}
	}
	return strings.Join(lines, "")
}

func containsSensitiveWord(line string) bool {
	lower := strings.ToLower(line)
	for _, word := range []string{
		"psk",
		"password",
		"passwd",
		"token",
		"secret",
		"key",
		"device_secret",
		"signature",
	} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}
