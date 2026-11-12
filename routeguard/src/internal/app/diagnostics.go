package app

import (
	"context"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"time"

	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

type DiagnosticReport struct {
	Time   string         `json:"time"`
	Checks []ProfileCheck `json:"checks"`
}

type ProfileCheck struct {
	Profile string `json:"profile"`
	ID      string `json:"id"`
	Device  string `json:"device,omitempty"`
	Address string `json:"address,omitempty"`
	Result  string `json:"result"`
	Packets uint64 `json:"packets"`
	Detail  string `json:"detail"`
}

// diagnose runs under the operation lock, serially, without changing rules,
// refreshing lists, recovering journals, or stopping other services.
func (a *App) diagnose(ctx context.Context) (*DiagnosticReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	saved, exists, err := a.Store.Load()
	if err != nil {
		return nil, err
	}
	if !exists || saved.Active == nil || saved.Config == nil || !saved.Config.Settings.Enabled {
		return nil, fmt.Errorf("no active protection; use Save & Apply first")
	}

	pending, err := a.Store.Pending()
	if err != nil {
		return nil, err
	}

	if pending != nil || saved.Paused {
		return nil, fmt.Errorf("recovery or rollback is pending; use Save & Apply first")
	}

	current, err := a.System.Config(ctx)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(current, saved.Config) {
		return nil, fmt.Errorf("settings differ from active rules; use Save & Apply first")
	}

	offload, err := a.System.Offload(ctx)
	if err != nil {
		return nil, err
	}

	if offload {
		return nil, fmt.Errorf("flow offloading may bypass protection; disable it before testing")
	}

	if a.Prober == nil {
		return nil, fmt.Errorf("network diagnostics are unavailable")
	}

	health, err := a.Firewall.Check(ctx, saved.Active)
	if err != nil {
		return nil, err
	}

	if !health.Matches {
		return nil, fmt.Errorf("active firewall rules could not be verified: %s", health.Reason)
	}

	report := &DiagnosticReport{Time: time.Now().UTC().Format(time.RFC3339), Checks: []ProfileCheck{}}
	attempts := 0

	for _, profile := range saved.Config.Services {
		row := ProfileCheck{Profile: profile.Name(), ID: profile.ID}

		if !profile.Enabled {
			row.Result, row.Detail = "skipped", "Profile is disabled"
			report.Checks = append(report.Checks, row)
			continue
		}

		targets := saved.Active.ProbeTargets[profile.ID]
		if len(targets) == 0 {
			row.Result, row.Detail = "skipped", "No host test targets saved. Update addresses; CIDR-only or fully exempt profiles have no test target."
			report.Checks = append(report.Checks, row)
			continue
		}

		// Also enforce the sampling limit when reading existing state files.
		for _, target := range targets[:min(2, len(targets))] {
			for _, device := range saved.Active.Plan.Devices {
				check := row
				check.Device, check.Address = device, target.String()
				check.Result, check.Detail = "inconclusive", "Test budget exhausted"
				if attempts < 32 && ctx.Err() == nil {
					attempts++
					check = a.checkTarget(ctx, saved.Active, check, target)
				}
				report.Checks = append(report.Checks, check)
			}
		}
	}

	return report, nil
}

func (a *App) checkTarget(ctx context.Context, snapshot *policy.Snapshot, row ProfileCheck, target netip.Addr) ProfileCheck {
	row.Result = "inconclusive"
	contains := func(prefix netip.Prefix) bool { return prefix.Contains(target) }
	if !target.IsValid() || !slices.ContainsFunc(snapshot.Plan.Targets, contains) || slices.ContainsFunc(snapshot.Plan.Exempt, contains) {
		row.Detail = "Test address is absent from the protected set or is exempt"
		return row
	}

	if !a.System.DeviceExists(row.Device) {
		row.Detail = "Selected uplink is unavailable"
		return row
	}

	counter := "output4"
	if target.Is6() {
		counter = "output6"
	}

	before, err := a.Firewall.Check(ctx, snapshot)
	if err != nil || !before.Matches {
		row.Detail = "Cannot verify firewall before the probe"
		return row
	}

	baseline, present := before.Counters[counter]
	if !present {
		row.Detail = "Required output counter is missing"
		return row
	}

	probeErr := a.Prober.Connect(ctx, row.Device, target)
	if probeErr == nil {
		row.Result, row.Detail = "reachable", "TCP port 443 is reachable; no block demonstrated. Local traffic redirection may affect the path."
		return row
	}
	after, err := a.Firewall.Check(ctx, snapshot)
	if err != nil || !after.Matches {
		row.Detail = "Cannot verify firewall after the probe"
		return row
	}

	row.Packets, present = packetDelta(baseline, after.Counters, counter)
	if !present {
		row.Detail = "Counter disappeared or reset during the probe"
		return row
	}

	if row.Packets > 0 {
		row.Result = "drop_observed"
		row.Detail = "Connection failed and RouteGuard dropped packets. Shared counters may include other traffic."
	} else {
		row.Detail = "Connection failed without a RouteGuard counter increase; routing, another firewall or the remote host may be responsible."
	}

	return row
}

func packetDelta(before nft.Counter, after map[string]nft.Counter, name string) (uint64, bool) {
	value, exists := after[name]
	if !exists || value.Packets < before.Packets {
		return 0, false
	}

	return value.Packets - before.Packets, true
}
