package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/nft"
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
	a.firewall.EXPECT().Check(gomock.Any(), hasPlan(first)).Return(nft.Health{Present: true, Matches: true}, nil)
	a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil)
	if _, err := a.Execute(ctx, "refresh"); err != nil {
		t.Fatal(err)
	}

	a.expectAvailableWAN()
	gomock.InOrder(
		a.scheduler.EXPECT().Check(gomock.Any(), "").Return(nil),
		a.firewall.EXPECT().Check(gomock.Any(), hasPlan(first)).Return(nft.Health{Present: true}, nil),
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(first)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(first)).Return(nil),
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
	if _, err := a.Execute(ctx, "rollback"); err != nil {
		t.Fatal(err)
	}

	if _, err := a.Execute(ctx, "refresh"); err != nil {
		t.Fatal(err)
	}

	gomock.InOrder(
		a.firewall.EXPECT().Validate(gomock.Any(), hasPlan(first)).Return(nil),
		a.firewall.EXPECT().Install(gomock.Any(), hasPlan(first)).Return(nil),
		a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
	)
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
