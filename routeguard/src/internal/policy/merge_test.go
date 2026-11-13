package policy

import (
	"context"
	_ "embed"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/policy/mock"
	"github.com/ZeeeUs/RouteGuard/internal/sources"
	"go.uber.org/mock/gomock"
)

//go:embed testdata/domains-prefixed.txt
var prefixedDomains string

//go:embed testdata/domains-plain.txt
var plainDomains string

func TestCompilerMergesRemoteListsAndManualDomains(t *testing.T) {
	loader := mock.NewMockSources(gomock.NewController(t))
	urls := []string{"https://first.example/domains", "https://second.example/domains"}
	for index, fixture := range []string{prefixedDomains, plainDomains} {
		loader.EXPECT().Domains(gomock.Any(), urls[index]).
			DoAndReturn(func(context.Context, string) (sources.DomainSet, int, error) {
				return sources.Parse(strings.NewReader(fixture))
			})
	}

	server := netip.MustParseAddr("192.0.2.53")
	for _, name := range []string{"example.org", "api.example.org", "cdn.example.org", "manual.example.org"} {
		// Each unique name is queried exactly once per family, despite overlaps.
		loader.EXPECT().Addresses(gomock.Any(), server, name, "ip4").
			Return([]netip.Prefix{netip.MustParsePrefix("192.0.2.10/32")}, nil)
		loader.EXPECT().Addresses(gomock.Any(), server, name, "ip6").
			Return([]netip.Prefix{netip.MustParsePrefix("2001:db8::10/128")}, nil)
	}

	cfg := &config.Config{
		Settings: config.Settings{Enabled: true, IPv6: "selective", DNS: server.String(), Devices: []string{"wan"}},
		Services: []config.Service{{
			ID: "merged", Enabled: true, URLs: urls,
			Domains: []string{"Example.org.", "manual.example.org"},
		}},
	}
	result, err := (Compiler{Sources: loader}).Compile(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	profile := result.Services[0]
	if profile.Domains != 4 || profile.Skipped != 2 || profile.IPv4 != 1 || profile.IPv6 != 1 {
		t.Fatalf("unexpected merged summary: %+v", profile)
	}

	if len(result.Plan.Targets) != 2 {
		t.Fatalf("shared IP addresses were not deduplicated: %+v", result.Plan.Targets)
	}
}

func TestCompilerDoesNotUsePartialListMerge(t *testing.T) {
	loader := mock.NewMockSources(gomock.NewController(t))
	failure := errors.New("second list unavailable")
	loader.EXPECT().Domains(gomock.Any(), "https://first.example/domains").
		DoAndReturn(func(context.Context, string) (sources.DomainSet, int, error) {
			return sources.Parse(strings.NewReader(prefixedDomains))
		})
	loader.EXPECT().Domains(gomock.Any(), "https://second.example/domains").Return(nil, 0, failure)

	cfg := testConfig()
	cfg.Services = []config.Service{{
		ID: "merged", Enabled: true,
		URLs: []string{"https://first.example/domains", "https://second.example/domains"},
	}}
	result, err := (Compiler{Sources: loader}).Compile(context.Background(), cfg)
	if !errors.Is(err, failure) || result != nil {
		t.Fatalf("partial merge accepted: %+v, %v", result, err)
	}
}
