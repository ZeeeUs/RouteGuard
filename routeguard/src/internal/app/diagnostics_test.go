package app

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"

	"github.com/ZeeeUs/RouteGuard/internal/app/mock"
	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/store"
	"go.uber.org/mock/gomock"
)

func TestDiagnosticsDistinguishesReachabilityDropsAndUncertainty(t *testing.T) {
	for _, tc := range []struct {
		name     string
		probeErr error
		packets  uint64
		matches  bool
		want     string
	}{
		{"reachable", nil, 10, true, "reachable"},
		{"drop observed", errors.New("timeout"), 12, true, "drop_observed"},
		{"timeout alone", errors.New("timeout"), 10, true, "inconclusive"},
		{"counter reset", errors.New("timeout"), 0, true, "inconclusive"},
		{"rules changed", errors.New("timeout"), 12, false, "inconclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := fixtureApp(t, t.TempDir())
			target := netip.MustParseAddr("192.0.2.10")
			active := snapshot(target.String())
			active.ProbeTargets = map[string][]netip.Addr{"manual": {target}}
			saved := store.State{Version: store.Version, Active: active, Config: manualConfig(target.String())}
			if err := a.Store.Save(saved); err != nil {
				t.Fatal(err)
			}

			prober := mock.NewMockProber(gomock.NewController(t))
			a.Prober = prober
			a.system.EXPECT().Config(gomock.Any()).Return(saved.Config, nil)
			a.expectAvailableWAN()
			health := nft.Health{Matches: true, Counters: map[string]nft.Counter{"output4": {Packets: 10}}}
			a.firewall.EXPECT().Check(gomock.Any(), active).Return(health, nil).Times(2)
			prober.EXPECT().Connect(gomock.Any(), "wan", target).Return(tc.probeErr)
			if tc.probeErr != nil {
				a.firewall.EXPECT().Check(gomock.Any(), active).Return(nft.Health{Matches: tc.matches, Counters: map[string]nft.Counter{"output4": {Packets: tc.packets}}}, nil)
			}
			// Execute exercises the queued worker's dispatch and JSON job storage.
			if _, err := a.Execute(context.Background(), "diagnose"); err != nil {
				t.Fatal(err)
			}
			job, _, err := a.jobStatus()
			if err != nil || job.Diagnostics == nil || len(job.Diagnostics.Checks) != 1 {
				t.Fatalf("missing report: %+v %v", job, err)
			}
			if got := job.Diagnostics.Checks[0].Result; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if !reflect.DeepEqual(saved, a.loadState(t)) {
				t.Fatal("diagnostics modified saved rules")
			}
		})
	}
}

func TestDiagnosticsDoesNotRecoverPendingChanges(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	saved := store.State{Version: store.Version, Active: snapshot("192.0.2.10"), Config: manualConfig("192.0.2.10")}
	if err := a.Store.Save(saved); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Prepare(saved); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Execute(context.Background(), "diagnose"); err == nil {
		t.Fatal("pending recovery accepted")
	}
	pending, err := a.Store.Pending()
	if err != nil || pending == nil {
		t.Fatal("diagnostics consumed recovery journal")
	}
}

func TestDiagnosticsRejectsUnappliedConfiguration(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	saved := store.State{Version: store.Version, Active: snapshot("192.0.2.10"), Config: manualConfig("192.0.2.10")}
	if err := a.Store.Save(saved); err != nil {
		t.Fatal(err)
	}
	a.system.EXPECT().Config(gomock.Any()).Return(manualConfig("192.0.2.11"), nil)
	if _, err := a.diagnose(context.Background()); err == nil {
		t.Fatal("unapplied configuration tested")
	}
}

func TestDiagnosticsNeverProbesExemptAddress(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	active := snapshot("192.0.2.10")
	active.Plan.Exempt = active.Plan.Targets
	row := a.checkTarget(context.Background(), active, ProfileCheck{Device: "wan"}, netip.MustParseAddr("192.0.2.10"))
	if row.Result != "inconclusive" {
		t.Fatalf("unexpected result: %+v", row)
	}
}
