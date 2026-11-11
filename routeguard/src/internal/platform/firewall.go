package platform

import (
	"context"

	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

type Firewall struct {
	Commands Runner
}

func (f Firewall) Validate(ctx context.Context, snapshot *policy.Snapshot) error {
	_, err := f.Commands.Run(ctx, nft.Batch(snapshot), "nft", "-c", "-f", "-")
	return err
}

func (f Firewall) Install(ctx context.Context, snapshot *policy.Snapshot) error {
	_, err := f.Commands.Run(ctx, nft.Batch(snapshot), "nft", "-f", "-")
	return err
}

func (f Firewall) Check(ctx context.Context, expected *policy.Snapshot) (nft.Health, error) {
	health := nft.Health{Counters: make(map[string]nft.Counter)}
	data, err := f.Commands.Run(ctx, "", "nft", "-j", "list", "tables")
	if err != nil {
		return health, err
	}

	health.Present, err = nft.TableExists(data)
	if err != nil {
		return health, err
	}

	if !health.Present {
		health.Matches = expected == nil
		if expected != nil {
			health.Reason = "RouteGuard table is absent"
		}

		return health, nil
	}

	// Do not use --terse: it omits set elements, which are part of protection.
	data, err = f.Commands.Run(ctx, "", "nft", "-j", "-n", "list", "table", "inet", "routeguard")
	if err != nil {
		return health, err
	}

	return nft.Inspect(data, expected)
}
