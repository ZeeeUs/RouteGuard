package app

import (
	"context"
	"fmt"

	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/platform"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

type Status struct {
	Loaded            bool                   `json:"loaded"`
	Present           bool                   `json:"table_present"`
	Reason            string                 `json:"reason,omitempty"`
	Counters          map[string]nft.Counter `json:"counters"`
	Paused            bool                   `json:"paused"`
	Busy              bool                   `json:"busy"`
	RecoveryPending   bool                   `json:"recovery_pending"`
	RollbackAvailable bool                   `json:"rollback_available"`
	IPv6              string                 `json:"ipv6"`
	Updated           string                 `json:"updated"`
	Devices           []string               `json:"devices"`
	DefaultRoutes     map[string][]string    `json:"default_route_devices"`
	Missing           []string               `json:"missing_devices"`
	Services          []policy.Summary       `json:"services"`
	Offload           bool                   `json:"offload"`
	Warnings          []string               `json:"warnings"`
	Schedule          platform.Schedule      `json:"schedule"`
	Job               Operation              `json:"job"`
}

func (a *App) Status(ctx context.Context) (Status, error) {
	result := Status{IPv6: "off", Devices: []string{}, Missing: []string{}, Services: []policy.Summary{}, Warnings: []string{}}
	job, busy, err := a.jobStatus()
	if err != nil {
		return result, err
	}
	result.Job, result.Busy = job, busy

	saved, exists, err := a.Store.Load()
	if err != nil {
		return result, err
	}

	pending, err := a.Store.Pending()
	if err != nil {
		return result, err
	}
	result.RecoveryPending = pending != nil

	health, err := a.Firewall.Check(ctx, saved.Active)
	if err != nil {
		return result, err
	}

	result.Present, result.Counters, result.Reason = health.Present, health.Counters, health.Reason
	result.Loaded = saved.Active != nil && health.Matches && pending == nil
	result.Paused, result.RollbackAvailable = saved.Paused, saved.Previous != nil
	if saved.MigrationWarning != "" {
		result.Warnings = append(result.Warnings, saved.MigrationWarning)
	}
	if active := saved.Active; active != nil {
		result.IPv6, result.Updated = active.IPv6, active.Updated
		result.Devices, result.Services = active.Plan.Devices, active.Services
		for _, device := range active.Plan.Devices {
			if !a.System.DeviceExists(device) {
				result.Missing = append(result.Missing, device)
			}
		}
	}

	if !exists {
		cfg, err := a.System.Config(ctx)
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		} else if cfg.Settings.Enabled {
			result.Reason = "enabled configuration has no saved rules; use Save & Apply to recover"
		}
	}

	result.Schedule = a.Scheduler.Status(ctx, schedule(saved))
	result.DefaultRoutes, err = a.System.Routes(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("routes: %s", err))
	}

	result.Offload, err = a.System.Offload(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("offload check: %s", err))
	}

	return result, nil
}
