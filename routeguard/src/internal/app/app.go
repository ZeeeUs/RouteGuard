// Package app coordinates operations; platform effects are injected at its boundary.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/platform"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

type StateStore interface {
	Load() (store.State, bool, error)
	Save(store.State) error
	Pending() (*store.State, error)
	Prepare(store.State) error
	Complete() error
}

type Firewall interface {
	Validate(context.Context, *policy.Snapshot) error
	Install(context.Context, *policy.Snapshot) error
	Check(context.Context, *policy.Snapshot) (nft.Health, error)
}

type System interface {
	Config(context.Context) (*config.Config, error)
	DeviceExists(string) bool
	Offload(context.Context) (bool, error)
	Routes(context.Context) (map[string][]string, error)
	Log(context.Context, string)
}

type Scheduler interface {
	Check(context.Context, string) error
	Update(context.Context, string) error
	Status(context.Context, string) platform.Schedule
}

type Compiler interface {
	Compile(context.Context, *config.Config) (*policy.Snapshot, error)
}

type App struct {
	Store      StateStore
	Firewall   Firewall
	System     System
	Scheduler  Scheduler
	Compiler   Compiler
	RunPath    string
	Executable string
}

func (a *App) compile(ctx context.Context, cfg *config.Config) (*policy.Snapshot, error) {
	if cfg.Settings.Enabled {
		offload, err := a.System.Offload(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not verify flow offloading: %w", err)
		}

		if offload {
			return nil, fmt.Errorf("disable software and hardware flow offloading in Firewall and apply it")
		}

		for _, device := range cfg.Settings.Devices {
			if !a.System.DeviceExists(device) {
				return nil, fmt.Errorf("WAN device absent: %q", device)
			}
		}
	}

	collection, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	return a.Compiler.Compile(collection, cfg)
}

func schedule(state store.State) string {
	if state.Active == nil || state.Paused {
		return ""
	}

	return state.Active.UpdateMinute
}
