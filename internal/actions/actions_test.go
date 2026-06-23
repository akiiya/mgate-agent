package actions

import "testing"

func TestUnknownActionRejected(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	_, _, err = registry.Validate("unknown.action", nil)
	if err == nil {
		t.Fatal("Validate() expected error")
	}
}

func TestCountryValidation(t *testing.T) {
	registry := mustRegistry(t)

	if _, _, err := registry.Validate("gateway.start", map[string]any{"country": "US"}); err != nil {
		t.Fatalf("country=US should pass: %v", err)
	}
	for _, country := range []string{"../../x", "US; rm -rf /", "$(reboot)", "`reboot`", "hello && reboot"} {
		t.Run(country, func(t *testing.T) {
			if _, _, err := registry.Validate("gateway.start", map[string]any{"country": country}); err == nil {
				t.Fatal("Validate() expected error")
			}
		})
	}
}

func TestWlanSwitchSafeBoundaries(t *testing.T) {
	registry := mustRegistry(t)

	validPSK8 := "12345678"
	validPSK64 := "1234567890123456789012345678901234567890123456789012345678901234"
	validSSID32 := "12345678901234567890123456789012"

	cases := []struct {
		name string
		args map[string]any
		ok   bool
	}{
		{name: "ssid empty", args: map[string]any{"ssid": "", "psk": validPSK8}},
		{name: "ssid max", args: map[string]any{"ssid": validSSID32, "psk": validPSK8}, ok: true},
		{name: "ssid too long", args: map[string]any{"ssid": validSSID32 + "x", "psk": validPSK8}},
		{name: "psk too short", args: map[string]any{"ssid": "home", "psk": "1234567"}},
		{name: "psk min", args: map[string]any{"ssid": "home", "psk": validPSK8}, ok: true},
		{name: "psk max", args: map[string]any{"ssid": "home", "psk": validPSK64}, ok: true},
		{name: "psk too long", args: map[string]any{"ssid": "home", "psk": validPSK64 + "x"}},
		{name: "unknown field", args: map[string]any{"ssid": "home", "psk": validPSK8, "extra": "x"}},
		{name: "psk unsafe", args: map[string]any{"ssid": "home", "psk": "hello&&reboot"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := registry.Validate("wlan.switch.safe", tc.args)
			if tc.ok && err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("Validate() expected error")
			}
		})
	}
}

func TestSensitiveArgsRedacted(t *testing.T) {
	registry := mustRegistry(t)
	spec, args, err := registry.Validate("wlan.switch.safe", map[string]any{"ssid": "home", "psk": "12345678"})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	redacted := spec.RedactedArgs(args)
	if redacted["psk"] != "***REDACTED***" {
		t.Fatalf("psk was not redacted: %+v", redacted)
	}
}

func mustRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	return registry
}
