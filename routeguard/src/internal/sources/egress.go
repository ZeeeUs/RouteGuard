package sources

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"time"
)

var nonPublicListNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func publicListAddress(addr netip.Addr) bool {
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.Zone() != "" {
		return false
	}
	for _, prefix := range nonPublicListNetworks {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func allowedPrivateList(path, host string, addr netip.Addr) (bool, error) {
	if path == "" || !addr.IsValid() || addr.Zone() != "" {
		return false, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()

	host = strings.TrimSuffix(strings.ToLower(host), ".")
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		record, _, _ := strings.Cut(scanner.Text(), "#")
		fields := strings.Fields(record)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return false, fmt.Errorf("invalid private list allowlist entry on line %d", line)
		}
		prefix, err := netip.ParsePrefix(fields[1])
		if err != nil {
			return false, fmt.Errorf("invalid private list allowlist prefix on line %d", line)
		}
		if strings.TrimSuffix(strings.ToLower(fields[0]), ".") == host && prefix.Contains(addr) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func listDialer(path string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, target string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}

		dialer := net.Dialer{
			Timeout: 5 * time.Second,
			// ControlContext receives the resolved address immediately before
			// connect. The check applies to each DNS answer and IP literal.
			ControlContext: func(_ context.Context, _, resolved string, _ syscall.RawConn) error {
				ip, _, err := net.SplitHostPort(resolved)
				if err != nil {
					return err
				}
				addr, err := netip.ParseAddr(ip)
				if err != nil {
					return err
				}
				addr = addr.Unmap()
				if publicListAddress(addr) {
					return nil
				}
				allowed, err := allowedPrivateList(path, host, addr)
				if err != nil {
					return err
				}
				if !allowed {
					return fmt.Errorf("remote list address is not public or explicitly allowed")
				}
				return nil
			},
		}

		return dialer.DialContext(ctx, network, target)
	}
}
