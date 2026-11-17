package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
	policymock "github.com/ZeeeUs/RouteGuard/internal/policy/mock"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

func TestApplyRefreshRepairRollbackRestore(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	ctx := context.Background()
	a.system.EXPECT().Config(gomock.Any()).Return(&config.Config{}, nil)
	if _, err := a.Execute(ctx, "restore"); err != nil {
		t.Fatalf("fresh install: %v", err)
	}

	first := snapshot("192.0.2.10")
	a.system.EXPECT().Config(gomock.Any()).Return(manualConfig("192.0.2.10"), nil)
	a.expectAvailableWAN()
	gomock.InOrder(
		a.scheduler.EXPECT().Check(gomock.Any(), "").Return(nil),
		a.firewall.EXPECT().Check(gomock.Any(), hasPlan(first)).Return(nft.Health{}, nil),
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(first)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(first)).Return(nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
	)
	if _, err := a.Execute(ctx, "apply"); err != nil {
		t.Fatal(err)
	}

	// Refresh must read the applied configuration from disk, without reading
	// pending UCI or reinstalling an unchanged table (which resets counters).
	a.expectAvailableWAN()
	a.scheduler.EXPECT().Check(gomock.Any(), "").Return(nil)
	a.firewall.EXPECT().Check(gomock.Any(), hasPlan(first)).Return(nft.Health{Present: true, Matches: true}, nil).Times(2)
	a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil)
	if _, err := a.Execute(ctx, "refresh"); err != nil {
		t.Fatal(err)
	}

	a.expectAvailableWAN()
	gomock.InOrder(
		a.firewall.EXPECT().Check(gomock.Any(), hasPlan(first)).Return(nft.Health{Present: true}, nil),
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(first)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(first)).Return(nil),
		a.scheduler.EXPECT().Check(gomock.Any(), "").Return(nil),
		a.firewall.EXPECT().Check(gomock.Any(), hasPlan(first)).Return(nft.Health{Present: true, Matches: true}, nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
	)
	message, err := a.Execute(ctx, "refresh")
	if err != nil || !strings.Contains(message, "Repaired") {
		t.Fatalf("damaged rules not repaired: %s, %v", message, err)
	}

	second := snapshot("192.0.2.20")
	a.system.EXPECT().Config(gomock.Any()).Return(manualConfig("192.0.2.20"), nil)
	a.expectAvailableWAN()
	gomock.InOrder(
		a.scheduler.EXPECT().Check(gomock.Any(), "").Return(nil),
		a.firewall.EXPECT().Check(gomock.Any(), hasPlan(second)).Return(nft.Health{Present: true}, nil),
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(second)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(second)).Return(nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
	)
	if _, err := a.Execute(ctx, "apply"); err != nil {
		t.Fatal(err)
	}

	before := a.loadState(t)
	a.system.EXPECT().Config(gomock.Any()).Return(manualConfig("0.0.0.0/0"), nil)
	a.expectAvailableWAN()
	if _, err := a.Execute(ctx, "apply"); err == nil {
		t.Fatal("invalid apply accepted")
	}

	if after := a.loadState(t); !reflect.DeepEqual(before, after) {
		t.Fatal("failed apply modified saved state")
	}

	gomock.InOrder(
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(first)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(first)).Return(nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
	)
	a.system.EXPECT().Offload(gomock.Any()).Return(false, nil)
	if _, err := a.Execute(ctx, "rollback"); err != nil {
		t.Fatal(err)
	}

	a.firewall.EXPECT().Check(gomock.Any(), hasPlan(first)).Return(nft.Health{Matches: true}, nil)
	if _, err := a.Execute(ctx, "refresh"); err != nil {
		t.Fatal(err)
	}

	gomock.InOrder(
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(first)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(first)).Return(nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
	)
	a.system.EXPECT().Offload(gomock.Any()).Return(false, nil)
	if _, err := a.Execute(ctx, "restore"); err != nil {
		t.Fatal(err)
	}

	saved := a.loadState(t)
	if !saved.Paused || !reflect.DeepEqual(saved.Active.Plan, first.Plan) {
		t.Fatal("rollback not preserved")
	}
}

func TestMissingSnapshotIsRecoveredFromExplicitConfig(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	want := snapshot("192.0.2.10")
	a.system.EXPECT().Config(gomock.Any()).Return(manualConfig("192.0.2.10"), nil)
	a.expectAvailableWAN()
	gomock.InOrder(
		a.scheduler.EXPECT().Check(gomock.Any(), "").Return(nil),
		a.firewall.EXPECT().Check(gomock.Any(), hasPlan(want)).Return(nft.Health{}, nil),
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(want)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(want)).Return(nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
	)
	if _, err := a.Execute(context.Background(), "restore"); err != nil {
		t.Fatal(err)
	}

	if saved := a.loadState(t); saved.Active == nil || !reflect.DeepEqual(saved.Active.Plan, want.Plan) {
		t.Fatal("enabled config was not recovered")
	}
}

func TestRefreshRestoresSavedRulesBeforeTryingUnavailableSources(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	cfg := manualConfig("192.0.2.10")
	cfg.Settings.DNS = "1.1.1.1"
	cfg.Services[0].Domains = []string{"service.example"}
	saved := store.State{Active: snapshot("192.0.2.10"), Config: cfg}
	if err := a.Store.Save(saved); err != nil {
		t.Fatal(err)
	}
	loader := policymock.NewMockSources(gomock.NewController(t))
	a.Compiler = policy.Compiler{Sources: loader}
	failure := errors.New("DNS unavailable")
	gomock.InOrder(
		a.firewall.EXPECT().Check(gomock.Any(), hasPlan(saved.Active)).Return(nft.Health{}, nil),
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(saved.Active)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(saved.Active)).Return(nil),
		a.system.EXPECT().Offload(gomock.Any()).Return(false, nil),
		a.system.EXPECT().DeviceExists("wan").Return(true),
		loader.EXPECT().Addresses(gomock.Any(), gomock.Any(), "service.example", "ip4").Return(nil, failure),
	)

	if _, err := a.Execute(context.Background(), "refresh"); !errors.Is(err, failure) || !strings.Contains(err.Error(), "saved firewall rules restored") {
		t.Fatalf("refresh outcome: %v", err)
	}
}

func TestRestoreReportsScheduleAndOffloadWarningsAfterInstalling(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	saved := store.State{Active: snapshot("192.0.2.10")}
	saved.Active.UpdateMinute = "15"
	if err := a.Store.Save(saved); err != nil {
		t.Fatal(err)
	}
	gomock.InOrder(
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(saved.Active)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(saved.Active)).Return(nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "15").Return(errors.New("cron offline")),
		a.system.EXPECT().Offload(gomock.Any()).Return(true, nil),
	)

	message, err := a.Execute(context.Background(), "restore")
	if err != nil || !strings.Contains(message, "Restored saved rules") ||
		!strings.Contains(message, "cron offline") || !strings.Contains(message, "flow offloading is active") {
		t.Fatalf("restore outcome: %s, %v", message, err)
	}
	job, _, err := a.jobStatus()
	if err != nil || job.State != "warning" {
		t.Fatalf("operation should report a warning: %+v, %v", job, err)
	}
}
