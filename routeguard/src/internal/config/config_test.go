package config

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed testdata/settings.uci
var settingsFixture string

//go:embed testdata/conflicting-field.uci
var conflictingFixture []byte

func TestParseTypedConfig(t *testing.T) {
	cfg, err := Parse([]byte(settingsFixture))
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Settings.Enabled || cfg.Settings.Devices[0] != "pppoe-wan" ||
		cfg.Services[0].Label != "Alice's # service" || cfg.Services[0].Domains[0] != "example.org" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	named := strings.Replace(settingsFixture, "config service", "config service 'stable_id'", 1)
	cfg, err = Parse([]byte(named))
	if err != nil || cfg.Services[0].ID != "stable_id" {
		t.Fatalf("service ID not preserved: %+v, %v", cfg, err)
	}

	if _, err := Parse(conflictingFixture); err == nil {
		t.Fatal("accepted conflicting option/list types")
	}
}

func TestBooleanValuesCannotSilentlyDisable(t *testing.T) {
	for _, value := range []string{"1", "true", "yes", "on", "enabled", "TRUE"} {
		t.Run(value, func(t *testing.T) {
			text := strings.Replace(settingsFixture, "enabled '1'", "enabled '"+value+"'", 1)
			cfg, err := Parse([]byte(text))
			if err != nil || !cfg.Settings.Enabled {
				t.Fatalf("enabled=%s: %+v, %v", value, cfg, err)
			}
		})
	}

	for _, value := range []string{"0", "false", "no", "off", "disabled"} {
		got, err := ParseBool(value)
		if err != nil || got {
			t.Fatalf("disabled=%s: %v, %v", value, got, err)
		}
	}

	for _, value := range []string{"", "2", "tru", "falsee"} {
		text := strings.Replace(settingsFixture, "enabled '1'", "enabled '"+value+"'", 1)
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("accepted malformed boolean %q", value)
		}
	}
}

func TestRejectUnknownFieldsAndMalformedUCI(t *testing.T) {
	for _, text := range []string{
		strings.Replace(settingsFixture, "enabled", "enabeld", 1),
		strings.Replace(settingsFixture, "list wan_device", "option wan_device", 1),
		strings.Replace(settingsFixture, "config service", "config arbitrary", 1),
		strings.Replace(settingsFixture, "'main'", "'other'", 1),
		settingsFixture + "option enabled 'tru'\n",
		settingsFixture + "option preset 'old'\n",
		settingsFixture + "config settings 'main'\n",
		settingsFixture + "option label 'unterminated\n",
	} {
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("accepted invalid configuration: %q", text)
		}
	}
}

func TestMinute(t *testing.T) {
	for input, want := range map[string]string{"": "", "00": "0", "05": "5", "59": "59"} {
		got, err := Minute(input)
		if err != nil || got != want {
			t.Errorf("Minute(%q)=%q,%v", input, got, err)
		}
	}

	for _, input := range []string{"60", "-1", "*", "+1", "000", "1\n"} {
		if _, err := Minute(input); err == nil {
			t.Errorf("accepted minute %q", input)
		}
	}
}
