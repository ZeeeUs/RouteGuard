package policy

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"

	"github.com/ZeeeUs/RouteGuard/internal/config"
)

var deviceName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.:-]{0,14}$`)

func ParsePrefixes(values []string, ipv6, exempt bool) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(values))

	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			addr, addrErr := netip.ParseAddr(value)
			if addrErr != nil || addr.Zone() != "" {
				return nil, fmt.Errorf("invalid IP address or CIDR %q", value)
			}

			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}

		if prefix != prefix.Masked() || prefix.Bits() == 0 || prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("invalid network prefix %q", value)
		}

		if exempt && prefix.Bits() != prefix.Addr().BitLen() {
			return nil, fmt.Errorf("tunnel exceptions must be single addresses")
		}

		if prefix.Addr().Is6() && !ipv6 {
			return nil, fmt.Errorf("IPv6 addresses require selective IPv6 mode")
		}

		out = append(out, prefix)
	}

	return SortPrefixes(out), nil
}

func SortPrefixes(values []netip.Prefix) []netip.Prefix {
	slices.SortFunc(values, func(a, b netip.Prefix) int {
		if order := a.Addr().Compare(b.Addr()); order != 0 {
			return order
		}

		return a.Bits() - b.Bits()
	})

	return slices.Compact(values)
}

func ValidateDevices(devices []string) error {
	if len(devices) == 0 || len(devices) > 32 {
		return fmt.Errorf("specify between 1 and 32 Linux WAN devices")
	}

	for _, device := range devices {
		if !deviceName.MatchString(device) {
			return fmt.Errorf("invalid WAN device name %q", device)
		}
	}

	return nil
}

func validateSettings(options config.Settings) (netip.Addr, []netip.Prefix, error) {
	if options.IPv6 != "off" && options.IPv6 != "selective" {
		return netip.Addr{}, nil, fmt.Errorf("select an IPv6 mode: off or selective")
	}

	if err := ValidateDevices(options.Devices); err != nil {
		return netip.Addr{}, nil, err
	}

	var server netip.Addr
	if options.DNS != "" {
		address, err := netip.ParseAddr(options.DNS)
		if err != nil || address.Zone() != "" {
			return netip.Addr{}, nil, fmt.Errorf("DNS resolver must be an unscoped IP address")
		}

		server = address
	}

	if len(options.Exempt) > 128 {
		return netip.Addr{}, nil, fmt.Errorf("limit: 128 tunnel endpoint exceptions")
	}

	exempt, err := ParsePrefixes(options.Exempt, options.IPv6 == "selective", true)
	return server, exempt, err
}
