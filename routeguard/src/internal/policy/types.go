// Package policy builds deterministic filtering plans without modifying the system.
package policy

import "net/netip"

const (
	MaxServices  = 8
	MaxAddresses = 4096
)

type Summary struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Domains int    `json:"domains"`
	Skipped int    `json:"skipped"`
	IPv4    int    `json:"ipv4"`
	IPv6    int    `json:"ipv6"`
}

type Plan struct {
	Devices []string       `json:"devices"`
	Targets []netip.Prefix `json:"targets"`
	Exempt  []netip.Prefix `json:"exempt"`
}

type Snapshot struct {
	Plan         Plan      `json:"plan"`
	IPv6         string    `json:"ipv6"`
	Services     []Summary `json:"services"`
	Updated      string    `json:"updated"`
	UpdateMinute string    `json:"update_minute,omitempty"`
}
