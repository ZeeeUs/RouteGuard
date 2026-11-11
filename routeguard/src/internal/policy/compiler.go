package policy

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/sources"
)

type Sources interface {
	Domains(context.Context, string) (sources.DomainSet, int, error)
	Addresses(context.Context, netip.Addr, string, string) ([]netip.Prefix, error)
}

type Compiler struct {
	Sources Sources
	Now     func() time.Time
}

// collection uses one bit per service to share DNS results without duplicating buffers.
type collection struct {
	domains   map[string]uint8
	targets   map[netip.Prefix]uint8
	summaries []Summary
}

func (c Compiler) Compile(ctx context.Context, cfg *config.Config) (*Snapshot, error) {
	if !cfg.Settings.Enabled {
		return nil, nil
	}

	server, exempt, err := validateSettings(cfg.Settings)
	if err != nil {
		return nil, err
	}

	minute, err := config.Minute(cfg.Settings.UpdateMinute)
	if err != nil {
		return nil, err
	}

	items := collection{domains: make(map[string]uint8), targets: make(map[netip.Prefix]uint8)}
	for _, profile := range cfg.Services {
		if profile.Enabled {
			if err := c.collectService(ctx, &items, profile, cfg.Settings.IPv6 == "selective", server); err != nil {
				return nil, fmt.Errorf("service %s: %w", profile.ID, err)
			}
		}
	}

	if len(items.summaries) == 0 {
		return nil, fmt.Errorf("enable at least one service, or disable RouteGuard globally")
	}

	if err := c.resolveDomains(ctx, &items, server, cfg.Settings.IPv6 == "selective"); err != nil {
		return nil, err
	}

	targets, err := items.finish()
	if err != nil {
		return nil, err
	}

	devices := slices.Clone(cfg.Settings.Devices)
	slices.Sort(devices)
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}

	return &Snapshot{
		Plan:         Plan{Devices: slices.Compact(devices), Targets: targets, Exempt: exempt},
		IPv6:         cfg.Settings.IPv6,
		Services:     items.summaries,
		Updated:      now().UTC().Format(time.RFC3339),
		UpdateMinute: minute,
	}, nil
}

func (c Compiler) collectService(ctx context.Context, items *collection, profile config.Service, ipv6 bool, server netip.Addr) error {
	if len(items.summaries) == MaxServices || len(profile.URLs) > 4 || len(profile.IPs) > 512 || len(profile.Domains) > sources.MaxDomains {
		return fmt.Errorf("limit: 8 services, 4 URLs, 256 domains and 512 IP entries per service")
	}

	if (len(profile.Domains) > 0 || len(profile.URLs) > 0) && !server.IsValid() {
		return fmt.Errorf("configure a DNS resolver when using domain names or remote lists")
	}

	domains, skipped, err := c.collectDomains(ctx, profile)
	if err != nil {
		return err
	}

	mask := uint8(1 << len(items.summaries))
	for name := range domains {
		items.domains[name] |= mask
	}

	if len(items.domains) > sources.MaxDomains {
		return fmt.Errorf("limit: %d distinct domains", sources.MaxDomains)
	}

	manual, err := ParsePrefixes(profile.IPs, ipv6, false)
	if err != nil {
		return err
	}

	for _, prefix := range manual {
		if err := items.add(prefix, mask); err != nil {
			return err
		}
	}

	items.summaries = append(items.summaries, Summary{
		ID: profile.ID, Label: profile.Name(), Domains: len(domains), Skipped: skipped,
	})

	return nil
}

func (c Compiler) collectDomains(ctx context.Context, profile config.Service) (sources.DomainSet, int, error) {
	domains := make(sources.DomainSet)
	for _, value := range profile.Domains {
		name, err := sources.DomainName(value)
		if err != nil {
			return nil, 0, err
		}

		domains[name] = struct{}{}
	}

	skipped := 0
	for _, url := range profile.URLs {
		names, ignored, err := c.Sources.Domains(ctx, url)
		if err != nil {
			return nil, 0, err
		}

		skipped += ignored
		for name := range names {
			domains[name] = struct{}{}
		}
	}

	return domains, skipped, nil
}

func (c Compiler) resolveDomains(ctx context.Context, items *collection, server netip.Addr, ipv6 bool) error {
	names := make([]string, 0, len(items.domains))
	for name := range items.domains {
		names = append(names, name)
	}
	slices.Sort(names)

	families := []string{"ip4"}
	if ipv6 {
		families = append(families, "ip6")
	}

	// Sequential queries bound the number of live DNS buffers on small routers.
	for _, name := range names {
		for _, family := range families {
			resolved, err := c.Sources.Addresses(ctx, server, name, family)
			if err != nil {
				return err
			}

			for _, prefix := range resolved {
				if err := items.add(prefix, items.domains[name]); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (items *collection) add(prefix netip.Prefix, mask uint8) error {
	items.targets[prefix] |= mask
	if len(items.targets) > MaxAddresses {
		return fmt.Errorf("limit: %d distinct address entries", MaxAddresses)
	}

	return nil
}

func (items *collection) finish() ([]netip.Prefix, error) {
	targets := make([]netip.Prefix, 0, len(items.targets))
	for prefix, mask := range items.targets {
		targets = append(targets, prefix)
		for index := range items.summaries {
			if mask&(1<<index) == 0 {
				continue
			}

			if prefix.Addr().Is4() {
				items.summaries[index].IPv4++
			} else {
				items.summaries[index].IPv6++
			}
		}
	}

	for _, profile := range items.summaries {
		if profile.IPv4+profile.IPv6 == 0 {
			return nil, fmt.Errorf("%s: no usable addresses; previous rules retained", profile.Label)
		}
	}

	return SortPrefixes(targets), nil
}
