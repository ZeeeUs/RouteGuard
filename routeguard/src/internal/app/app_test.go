package app

import (
	"net/netip"
	"path/filepath"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/app/mock"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

type testApp struct {
	*App
	firewall  *mock.MockFirewall
	system    *mock.MockSystem
	scheduler *mock.MockScheduler
}

func fixtureApp(t *testing.T, directory string) *testApp {
	t.Helper()
	ctrl := gomock.NewController(t)
	firewall := mock.NewMockFirewall(ctrl)
	system := mock.NewMockSystem(ctrl)
	scheduler := mock.NewMockScheduler(ctrl)

	// Logging is incidental in these tests; all system mutations and reads
	// require explicit expectations in the scenario that exercises them.
	system.EXPECT().Log(gomock.Any(), gomock.Any()).AnyTimes()

	return &testApp{
		App: &App{
			Store:     store.Files{Path: filepath.Join(directory, "state.json")},
			Firewall:  firewall,
			System:    system,
			Scheduler: scheduler,
			Compiler:  policy.Compiler{},
			RunPath:   filepath.Join(directory, "run"),
		},
		firewall: firewall, system: system, scheduler: scheduler,
	}
}

func manualConfig(ip string) *config.Config {
	return &config.Config{
		Settings: config.Settings{Enabled: true, IPv6: "off", Devices: []string{"wan"}},
		Services: []config.Service{{ID: "manual", Enabled: true, IPs: []string{ip}}},
	}
}

func snapshot(ip string) *policy.Snapshot {
	return &policy.Snapshot{
		Plan: policy.Plan{Devices: []string{"wan"}, Targets: []netip.Prefix{netip.MustParsePrefix(ip + "/32")}, Exempt: []netip.Prefix{}},
		IPv6: "off", Updated: "previous",
	}
}

func hasPlan(want *policy.Snapshot) gomock.Matcher {
	return gomock.Cond(func(got *policy.Snapshot) bool {
		return got != nil && reflect.DeepEqual(got.Plan, want.Plan)
	})
}

func (a *testApp) expectAvailableWAN() {
	a.system.EXPECT().Offload(gomock.Any()).Return(false, nil)
	a.system.EXPECT().DeviceExists("wan").Return(true)
}

func (a *testApp) loadState(t *testing.T) store.State {
	t.Helper()
	saved, exists, err := a.Store.Load()
	if err != nil || !exists {
		t.Fatalf("saved state unavailable: %v", err)
	}

	return saved
}
