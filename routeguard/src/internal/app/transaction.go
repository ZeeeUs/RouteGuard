package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ZeeeUs/RouteGuard/internal/store"
)

// The durable journal records intent before touching the kernel. An interrupted
// operation rolls forward on the next command or boot, even without DNS/network.
func (a *App) publish(ctx context.Context, next, previous store.State) error {
	if err := a.Firewall.Validate(ctx, next.Active); err != nil {
		return err
	}

	next.Version = store.Version
	next.Generation = previous.Generation + 1
	if err := a.Store.Prepare(next); err != nil {
		return err
	}

	if err := a.commit(ctx, next); err != nil {
		// Persist the rollback intent BEFORE restoring previous rules. Recovery
		// must not resurrect a failed target after a second interruption.
		if prepareErr := a.Store.Prepare(previous); prepareErr != nil {
			return errors.Join(err, fmt.Errorf("recovery pending: %w", prepareErr))
		}

		rollbackErr := a.commit(context.WithoutCancel(ctx), previous)
		return errors.Join(err, rollbackErr)
	}

	return nil
}

func (a *App) commit(ctx context.Context, target store.State) error {
	if err := a.Firewall.Install(ctx, target.Active); err != nil {
		return err
	}

	if err := a.Store.Save(target); err != nil {
		return err
	}

	return a.Store.Complete()
}

func (a *App) recover(ctx context.Context) error {
	target, err := a.Store.Pending()
	if err != nil || target == nil {
		return err
	}

	if err := a.Firewall.Validate(ctx, target.Active); err != nil {
		return fmt.Errorf("recovery validation failed; journal retained: %w", err)
	}

	if err := a.commit(ctx, *target); err != nil {
		return fmt.Errorf("recovery incomplete; journal retained: %w", err)
	}

	return a.Scheduler.Update(ctx, schedule(*target))
}
