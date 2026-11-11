package app

import (
	"context"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/platform"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

func TestStatusDetectsDriftAndMissingDevices(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	saved := store.State{Version: store.Version, Active: snapshot("192.0.2.10")}
	if err := a.Store.Save(saved); err != nil {
		t.Fatal(err)
	}

	a.firewall.EXPECT().Check(gomock.Any(), saved.Active).Return(nft.Health{Present: true}, nil)
	a.system.EXPECT().DeviceExists("wan").Return(false)
	a.system.EXPECT().Routes(gomock.Any()).Return(map[string][]string{"-4": {"wan"}, "-6": {}}, nil)
	a.system.EXPECT().Offload(gomock.Any()).Return(false, nil)
	a.scheduler.EXPECT().Status(gomock.Any(), "").Return(platform.Schedule{})

	status, err := a.Status(context.Background())
	if err != nil || status.Loaded || !status.Present || !reflect.DeepEqual(status.Missing, []string{"wan"}) {
		t.Fatalf("misleading status: %+v, %v", status, err)
	}
}
