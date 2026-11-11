// Package config decodes UCI into the typed, validated application configuration.
package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Config struct {
	Settings Settings  `json:"settings"`
	Services []Service `json:"services"`
}

type Settings struct {
	Enabled      bool     `json:"enabled"`
	IPv6         string   `json:"ipv6"`
	DNS          string   `json:"dns"`
	UpdateMinute string   `json:"update_minute"`
	Devices      []string `json:"wan_device"`
	Exempt       []string `json:"exempt_ip"`
}

type Service struct {
	ID      string   `json:"id"`
	Enabled bool     `json:"enabled"`
	Label   string   `json:"label"`
	URLs    []string `json:"url"`
	Domains []string `json:"domain"`
	IPs     []string `json:"ip"`
}

func ParseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "1", "on", "true", "yes", "enabled":
		return true, nil
	case "0", "off", "false", "no", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func Minute(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	minute, err := strconv.Atoi(value)
	if err != nil || minute < 0 || minute > 59 || len(value) > 2 || strings.Trim(value, "0123456789") != "" {
		return "", fmt.Errorf("update minute must be a whole number from 0 to 59")
	}

	return strconv.Itoa(minute), nil
}

func (s Service) Name() string {
	if s.Label != "" {
		return s.Label
	}

	return s.ID
}
