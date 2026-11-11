package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

func (a *App) perform(ctx context.Context, action string) (string, error) {
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
		if saved.Paused || saved.Active == nil {
			return "Refresh skipped: no active rules or updates paused", nil
		}
	}

	cfg := saved.Config
	if action == "apply" {
		cfg, err = a.System.Config(ctx)
		if err != nil {
			return "", err
		}
	}

	if cfg == nil {
		return "", fmt.Errorf("no applied configuration; use Save & Apply")
	}

	return a.apply(ctx, saved, cfg)
}

func (a *App) apply(ctx context.Context, saved store.State, cfg *config.Config) (string, error) {
	updated, err := a.compile(ctx, cfg)
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

	if err := a.Scheduler.Update(ctx, schedule(next)); err != nil {
		return "", fmt.Errorf("%s; schedule failed: %w", message, err)
	}

	return message, nil
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

		message, err := a.apply(ctx, saved, cfg)
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

	return "Restored saved rules", a.Scheduler.Update(ctx, schedule(saved))
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

	return "Rolled back; scheduled refresh paused until Save & Apply", a.Scheduler.Update(ctx, "")
}
