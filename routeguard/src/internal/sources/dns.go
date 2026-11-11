package sources

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

var excludedDNS = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}
var syntheticDNSRange = netip.MustParsePrefix("198.18.0.0/15")

func ResolverFor(server netip.Addr) *net.Resolver {
	address := net.JoinHostPort(server.String(), "53")
	return &net.Resolver{
		PreferGo:     true,
		StrictErrors: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, address)
		},
	}
}

func resolvedPrefixes(name string, addresses []netip.Addr, family string) ([]netip.Prefix, error) {
	var result []netip.Prefix

	for _, address := range addresses {
		address = address.Unmap()
		if syntheticDNSRange.Contains(address) {
			return nil, fmt.Errorf("resolver returned a synthetic address for %s; keeping previous rules", name)
		}

		if address.Is4() != (family == "ip4") || !address.IsGlobalUnicast() || address.IsPrivate() {
			continue
		}

		allowed := true
		for _, prefix := range excludedDNS {
			if prefix.Contains(address) {
				allowed = false
				break
			}
		}

		if allowed {
			result = append(result, netip.PrefixFrom(address, address.BitLen()))
		}
	}

	return result, nil
}

func resolve(ctx context.Context, resolver *net.Resolver, name, family string) ([]netip.Prefix, error) {
	query, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	addresses, err := resolver.LookupNetIP(query, family, name+".")
	if err != nil {
		var dnsError *net.DNSError
		if errors.As(err, &dnsError) && dnsError.IsNotFound {
			return nil, nil
		}

		return nil, fmt.Errorf("DNS lookup for %s (%s): %w", name, family, err)
	}

	return resolvedPrefixes(name, addresses, family)
}
