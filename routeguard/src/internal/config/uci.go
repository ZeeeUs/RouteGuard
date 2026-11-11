package config

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// words reads the quoted syntax emitted by `uci export`, without executing it.
func words(line string) ([]string, error) {
	var result []string

	for line = strings.TrimSpace(line); line != "" && line[0] != '#'; line = strings.TrimSpace(line) {
		var field strings.Builder

		for len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			switch line[0] {
			case '\'':
				value, tail, found := strings.Cut(line[1:], "'")
				if !found {
					return nil, fmt.Errorf("unterminated UCI quote")
				}

				field.WriteString(value)
				line = tail
			case '\\':
				if len(line) < 2 {
					return nil, fmt.Errorf("unterminated UCI escape")
				}

				field.WriteByte(line[1])
				line = line[2:]
			default:
				field.WriteByte(line[0])
				line = line[1:]
			}
		}

		result = append(result, field.String())
	}

	return result, nil
}

type decoder struct {
	config       Config
	section      string
	settingsSeen bool
	ids          map[string]bool
}

func Parse(data []byte) (*Config, error) {
	d := decoder{ids: make(map[string]bool)}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	line := 0

	for scanner.Scan() {
		line++
		fields, err := words(scanner.Text())
		if err == nil {
			err = d.line(fields)
		}

		if err != nil {
			return nil, fmt.Errorf("UCI line %d: %w", line, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if !d.settingsSeen {
		return nil, fmt.Errorf("expected a settings section named main")
	}

	return &d.config, nil
}

func (d *decoder) line(fields []string) error {
	if len(fields) == 0 || fields[0] == "package" {
		return nil
	}

	switch fields[0] {
	case "config":
		return d.startSection(fields)
	case "option", "list":
		if len(fields) != 3 || d.section == "" {
			return fmt.Errorf("invalid UCI field")
		}

		return d.field(fields[0], fields[1], fields[2])
	default:
		return fmt.Errorf("invalid UCI directive %q", fields[0])
	}
}

func (d *decoder) startSection(fields []string) error {
	if len(fields) < 2 || len(fields) > 3 {
		return fmt.Errorf("invalid UCI section")
	}

	d.section = fields[1]
	switch d.section {
	case "settings":
		if len(fields) != 3 || fields[2] != "main" || d.settingsSeen {
			return fmt.Errorf("expected a single settings section named main")
		}

		d.settingsSeen = true
	case "service":
		id := fmt.Sprintf("@service[%d]", len(d.config.Services))
		if len(fields) == 3 {
			id = fields[2]
		}

		if d.ids[id] {
			return fmt.Errorf("duplicate service ID %q", id)
		}

		d.ids[id] = true
		d.config.Services = append(d.config.Services, Service{ID: id})
	default:
		return fmt.Errorf("unknown section %q", d.section)
	}

	return nil
}

func (d *decoder) field(kind, key, value string) error {
	var option *string
	var list *[]string
	var enabled *bool

	if d.section == "settings" {
		s := &d.config.Settings
		switch key {
		case "enabled":
			enabled = &s.Enabled
		case "ipv6":
			option = &s.IPv6
		case "dns":
			option = &s.DNS
		case "update_minute":
			option = &s.UpdateMinute
		case "wan_device":
			list = &s.Devices
		case "exempt_ip":
			list = &s.Exempt
		}
	} else {
		s := &d.config.Services[len(d.config.Services)-1]
		switch key {
		case "enabled":
			enabled = &s.Enabled
		case "label":
			option = &s.Label
		case "url":
			list = &s.URLs
		case "domain":
			list = &s.Domains
		case "ip":
			list = &s.IPs
		case "preset":
			return fmt.Errorf("replace the old preset with explicitly configured targets")
		}
	}

	return assign(kind, key, value, option, list, enabled)
}

func assign(kind, key, value string, option *string, list *[]string, enabled *bool) error {
	switch {
	case enabled != nil && kind == "option":
		parsed, err := ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}

		*enabled = parsed
	case option != nil && kind == "option":
		*option = value
	case list != nil && kind == "list":
		*list = append(*list, value)
	default:
		return fmt.Errorf("unknown field or incorrect option/list type: %s %s", kind, key)
	}

	return nil
}
