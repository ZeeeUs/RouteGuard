package config

import (
	"encoding/json"
	"fmt"
	"slices"
)

// DecodeLegacy is confined to migration of the unversioned prototype state.
// Normal configuration decoding never uses an intermediate JSON representation.
func DecodeLegacy(data json.RawMessage) (*Config, error) {
	if string(data) == "null" || len(data) == 0 {
		return nil, nil
	}

	var parts []json.RawMessage
	if err := json.Unmarshal(data, &parts); err != nil || len(parts) != 2 {
		return nil, fmt.Errorf("invalid legacy configuration")
	}

	d := decoder{section: "settings", settingsSeen: true}
	if err := d.legacySection(parts[0]); err != nil {
		return nil, err
	}

	var profiles []json.RawMessage
	if err := json.Unmarshal(parts[1], &profiles); err != nil {
		return nil, err
	}

	for index, profile := range profiles {
		d.section = "service"
		d.config.Services = append(d.config.Services, Service{ID: fmt.Sprintf("@service[%d]", index)})
		if err := d.legacySection(profile); err != nil {
			return nil, err
		}
	}

	return &d.config, nil
}

func (d *decoder) legacySection(data json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		raw := fields[key]
		if len(raw) > 0 && raw[0] == '[' {
			var values []string
			if err := json.Unmarshal(raw, &values); err != nil {
				return err
			}

			for _, value := range values {
				if err := d.field("list", key, value); err != nil {
					return err
				}
			}
		} else {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}

			if err := d.field("option", key, value); err != nil {
				return err
			}
		}
	}

	return nil
}
