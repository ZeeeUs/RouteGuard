package policy

import (
	"context"
	"net/netip"
	"slices"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/policy/mock"
)

func TestRefreshRetainsAnEmptyDNSAnswerUntilItsGracePeriodExpires(t *testing.T) {
	dns := mock.NewMockSources(gomock.NewController(t))
	cfg := &config.Config{
		Settings: config.Settings{Enabled: true, IPv6: "off", Devices: []string{"wan"}, DNS: "1.1.1.1"},
		Services: []config.Service{{ID: "svc", Enabled: true, Domains: []string{"a.example", "b.example"}}},
	}
	server := netip.MustParseAddr(cfg.Settings.DNS)
	old := netip.MustParsePrefix("192.0.2.10/32")
	other := netip.MustParsePrefix("192.0.2.20/32")
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	compiler := Compiler{Sources: dns, Now: func() time.Time { return now }}

	dns.EXPECT().Addresses(gomock.Any(), server, "a.example", "ip4").Return([]netip.Prefix{old}, nil)
	dns.EXPECT().Addresses(gomock.Any(), server, "b.example", "ip4").Return([]netip.Prefix{other}, nil).Times(3)
	dns.EXPECT().Addresses(gomock.Any(), server, "a.example", "ip4").Return(nil, nil).Times(2)

	baseline, err := compiler.Compile(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	retained, err := compiler.Compile(context.Background(), cfg, baseline)
	if err != nil || !slices.Equal(retained.Plan.Targets, []netip.Prefix{old, other}) {
		t.Fatalf("refresh dropped a recently known address: %+v, %v", retained, err)
	}

	now = now.Add(negativeDNSGrace)
	expired, err := compiler.Compile(context.Background(), cfg, retained)
	if err != nil || !slices.Equal(expired.Plan.Targets, []netip.Prefix{other}) {
		t.Fatalf("refresh kept an expired address: %+v, %v", expired, err)
	}
}
