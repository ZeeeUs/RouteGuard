package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

func (a *App) perform(ctx context.Context, action, revision string) (string, error) {
	if action == "stop" {
		return "RouteGuard stopped", errors.Join(a.Store.Complete(), a.Scheduler.Update(ctx, ""), a.Firewall.Install(ctx, nil))
	}

	if err := a.recover(ctx); err != nil {
		return "", err
	}

	saved, exists, err := a.Store.Load()
	if err != nil {
		return "", err
	}

	repaired := false
	switch action {
	case "restore":
		return a.restore(ctx, saved, exists)
	case "recover":
		if exists {
			return "Saved state already exists", nil
		}

		return a.restore(ctx, saved, false)
	case "rollback":
		return a.rollback(ctx, saved)
	case "refresh":
		if saved.Active != nil {
			repaired, err = a.ensureSavedRules(ctx, saved.Active)
			if err != nil {
				return "", err
			}
			if saved.Paused && repaired {
				return a.completed(ctx, "Repaired saved rules; address updates remain paused", "", true)
			}
		}
		if saved.Paused || saved.Active == nil {
			return "Refresh skipped: no active rules or updates paused", nil
		}
	}

	cfg := saved.Config
	if action == "apply" {
		if revision == "" {
			cfg, err = a.System.Config(ctx)
		} else {
			cfg, err = a.System.ConfigAtRevision(ctx, revision)
		}
		if err != nil {
			return "", err
		}
	}

	if cfg == nil {
		return "", fmt.Errorf("no applied configuration; use Save & Apply")
	}

	var previous *policy.Snapshot
	if action == "refresh" {
		previous = saved.Active
	}
	message, err := a.apply(ctx, saved, cfg, previous)
	if action == "refresh" && repaired && err != nil && message == "" {
		return "", fmt.Errorf("saved firewall rules restored; address refresh failed: %w", err)
	}
	if action == "refresh" && repaired {
		message = "Repaired saved rules; " + message
	}
	return message, err
}

func (a *App) apply(ctx context.Context, saved store.State, cfg *config.Config, previous *policy.Snapshot) (string, error) {
	updated, err := a.compile(ctx, cfg, previous)
	if err != nil {
		return "", err
	}

	next := saved
	next.Active, next.Config, next.Paused = updated, cfg, false
	next.MigrationWarning = ""
	if err := a.Scheduler.Check(ctx, schedule(next)); err != nil {
		return "", err
	}

	message, err := a.reconcile(ctx, saved, next)
	if err != nil {
		return "", err
	}

	return a.completed(ctx, message, schedule(next), false)
}

func samePlan(a, b *policy.Snapshot) bool {
	return a == nil && b == nil || a != nil && b != nil && reflect.DeepEqual(a.Plan, b.Plan)
}

func (a *App) reconcile(ctx context.Context, saved, next store.State) (string, error) {
	unchanged := samePlan(saved.Active, next.Active)
	health, err := a.Firewall.Check(ctx, next.Active)
	if err != nil {
		return "", err
	}

	if unchanged && next.Active != nil {
		next.Active.Updated = saved.Active.Updated
	}

	if !unchanged && saved.Active != nil {
		next.Previous = saved.Active
	}

	if unchanged && health.Matches {
		if !reflect.DeepEqual(saved, next) {
			if err := a.Store.Save(next); err != nil {
				return "", err
			}
		}

		return "Rules unchanged; kernel state verified", nil
	}

	if err := a.publish(ctx, next, saved); err != nil {
		return "", err
	}

	if next.Active == nil {
		return "RouteGuard disabled", nil
	}

	if unchanged {
		return "Repaired RouteGuard rules", nil
	}

	return "Applied RouteGuard rules", nil
}

func (a *App) restore(ctx context.Context, saved store.State, exists bool) (string, error) {
	if !exists {
		cfg, err := a.System.Config(ctx)
		if err != nil {
			return "", err
		}

		if !cfg.Settings.Enabled {
			return "RouteGuard is not configured", nil
		}

		message, err := a.apply(ctx, saved, cfg, nil)
		if err != nil {
			return "", fmt.Errorf("enabled configuration has no saved rules; recovery will retry on interface-up, or use Save & Apply: %w", err)
		}

		return message, nil
	}

	if err := a.Firewall.Validate(ctx, saved.Active); err != nil {
		return "", err
	}

	if err := a.Firewall.Install(ctx, saved.Active); err != nil {
		return "", err
	}

	return a.completed(ctx, "Restored saved rules", schedule(saved), saved.Active != nil)
}

func (a *App) rollback(ctx context.Context, saved store.State) (string, error) {
	if saved.Previous == nil {
		return "", fmt.Errorf("no previous active ruleset available")
	}

	next := saved
	next.Active, next.Previous, next.Paused = saved.Previous, saved.Active, true
	if err := a.publish(ctx, next, saved); err != nil {
		return "", err
	}

	return a.completed(ctx, "Rolled back; scheduled refresh paused until Save & Apply", "", true)
}

func (a *App) ensureSavedRules(ctx context.Context, active *policy.Snapshot) (bool, error) {
	health, err := a.Firewall.Check(ctx, active)
	if err != nil || health.Matches {
		return false, err
	}

	if err := a.Firewall.Validate(ctx, active); err != nil {
		return false, err
	}
	if err := a.Firewall.Install(ctx, active); err != nil {
		return false, err
	}
	return true, nil
}

type operationWarning struct{ detail string }

func (w operationWarning) Error() string { return w.detail }

func (a *App) completed(ctx context.Context, message, minute string, checkOffload bool) (string, error) {
	var warnings []string
	if err := a.Scheduler.Update(ctx, minute); err != nil {
		warnings = append(warnings, "cron schedule could not be updated: "+err.Error())
	}
	if checkOffload {
		offload, err := a.System.Offload(ctx)
		switch {
		case err != nil:
			warnings = append(warnings, "could not verify flow offloading: "+err.Error())
		case offload:
			warnings = append(warnings, "flow offloading is active and can bypass forwarding rules")
		}
	}
	if len(warnings) > 0 {
		return message, operationWarning{detail: strings.Join(warnings, "; ")}
	}
	return message, nil
}
