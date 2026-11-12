package policy

import (
	"context"
	"net/netip"
	"reflect"
	"testing"

	"github.com/ZeeeUs/RouteGuard/internal/config"
)

func TestProbeTargetsAreBoundedDeterministicAndExcludeExceptions(t *testing.T) {
	cfg := &config.Config{
		Settings: config.Settings{Enabled: true, IPv6: "selective", Devices: []string{"wan"}, Exempt: []string{"192.0.2.1"}},
		Services: []config.Service{
			{ID: "one", Enabled: true, IPs: []string{"192.0.2.3", "192.0.2.1", "192.0.2.2", "2001:db8::2", "2001:db8::1"}},
			{ID: "two", Enabled: true, IPs: []string{"192.0.2.3"}},
			{ID: "cidr", Enabled: true, IPs: []string{"198.51.100.0/24"}},
		},
	}
	result, err := (Compiler{}).Compile(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]netip.Addr{
		"one": {netip.MustParseAddr("192.0.2.2"), netip.MustParseAddr("2001:db8::1")},
		"two": {netip.MustParseAddr("192.0.2.3")},
	}
	if !reflect.DeepEqual(result.ProbeTargets, want) {
		t.Fatalf("unexpected samples: %+v", result.ProbeTargets)
	}
}
