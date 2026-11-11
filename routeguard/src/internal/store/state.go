package store

import (
	"encoding/json"
	"fmt"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

const Version = 1

type State struct {
	Version          int              `json:"version"`
	Generation       uint64           `json:"generation"`
	Active           *policy.Snapshot `json:"active"`
	Previous         *policy.Snapshot `json:"previous"`
	Paused           bool             `json:"paused"`
	Config           *config.Config   `json:"config"`
	MigrationWarning string           `json:"migration_warning,omitempty"`
}

type Journal struct {
	Version int   `json:"version"`
	Target  State `json:"target"`
}

type Files struct {
	Path string
}

func (f Files) Load() (State, bool, error) {
	var raw json.RawMessage
	exists, err := ReadJSON(f.Path, &raw)
	if err != nil || !exists {
		return State{Version: Version}, exists, err
	}

	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return State{}, true, err
	}

	if header.Version == 0 {
		state, err := migrate(raw)
		return state, true, err
	}

	if header.Version != Version {
		return State{}, true, fmt.Errorf("unsupported state version %d; file preserved", header.Version)
	}

	var state State
	err = json.Unmarshal(raw, &state)
	return state, true, err
}

func (f Files) Save(state State) error {
	state.Version = Version
	return WriteJSON(f.Path, state)
}

func (f Files) Pending() (*State, error) {
	var journal Journal
	exists, err := ReadJSON(f.Path+".pending", &journal)
	if err != nil || !exists {
		return nil, err
	}

	if journal.Version != Version || journal.Target.Version != Version {
		return nil, fmt.Errorf("unsupported recovery journal version")
	}

	return &journal.Target, nil
}

func (f Files) Prepare(target State) error {
	target.Version = Version
	return WriteJSON(f.Path+".pending", Journal{Version: Version, Target: target})
}

func (f Files) Complete() error {
	return Remove(f.Path + ".pending")
}

type legacySnapshot struct {
	NFT          string           `json:"nft"`
	IPv6         string           `json:"ipv6"`
	Services     []policy.Summary `json:"services"`
	Updated      string           `json:"updated"`
	UpdateMinute string           `json:"update_minute"`
}

func migrate(raw json.RawMessage) (State, error) {
	var old struct {
		Active   *legacySnapshot `json:"active"`
		Previous *legacySnapshot `json:"previous"`
		Paused   bool            `json:"paused"`
		Config   json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		return State{}, err
	}

	state := State{Version: Version, Paused: old.Paused}
	var err error
	if state.Config, err = config.DecodeLegacy(old.Config); err != nil {
		state.Config, state.Paused = nil, true
		state.MigrationWarning = fmt.Sprintf("saved rules retained, but old configuration needs Save & Apply: %s", err)
	}

	for _, pair := range []struct {
		old    *legacySnapshot
		target **policy.Snapshot
	}{{old.Active, &state.Active}, {old.Previous, &state.Previous}} {
		if pair.old == nil {
			continue
		}

		plan, err := nft.DecodeLegacy(pair.old.NFT)
		if err != nil {
			return State{}, fmt.Errorf("legacy snapshot: %w", err)
		}

		*pair.target = &policy.Snapshot{
			Plan: plan, IPv6: pair.old.IPv6, Services: pair.old.Services,
			Updated: pair.old.Updated, UpdateMinute: pair.old.UpdateMinute,
		}
	}

	return state, nil
}
