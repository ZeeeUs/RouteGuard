package policy

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/policy/mock"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/sources"
)

func testConfig() *config.Config {
	return &config.Config{
		Settings: config.Settings{Enabled: true, IPv6: "selective", DNS: "8.8.8.8", Devices: []string{"wan", "wan"}},
		Services: []config.Service{
			{ID: "one", Enabled: true, Domains: []string{"example.org"}},
			{ID: "two", Enabled: true, URLs: []string{"https://example.org/list"}},
		},
	}
}

func TestCompilerSharesQueriesAndKeepsServiceIDs(t *testing.T) {
	loader := mock.NewMockSources(gomock.NewController(t))
	server := netip.MustParseAddr("8.8.8.8")
	loader.EXPECT().Domains(gomock.Any(), "https://example.org/list").
		Return(sources.DomainSet{"example.org": {}}, 2, nil)
	loader.EXPECT().Addresses(gomock.Any(), server, "example.org", "ip4").
		Return([]netip.Prefix{netip.MustParsePrefix("93.184.215.14/32")}, nil)
	loader.EXPECT().Addresses(gomock.Any(), server, "example.org", "ip6").
		Return([]netip.Prefix{netip.MustParsePrefix("2606:2800:220:1::1/128")}, nil)

	cfg := testConfig()
	result, err := (Compiler{Sources: loader}).Compile(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Plan.Devices) != 1 || len(result.Plan.Targets) != 2 {
		t.Fatalf("duplicate rules: %+v", result)
	}

	for index, service := range result.Services {
		if service.ID != cfg.Services[index].ID || service.IPv4 != 1 || service.IPv6 != 1 {
			t.Errorf("bad service summary: %+v", service)
		}
	}

	if len(cfg.Settings.Devices) != 2 {
		t.Fatal("compiler mutated input configuration")
	}
}

func TestCompilerFailsWithoutPublishingPartialTargets(t *testing.T) {
	for _, phase := range []string{"download", "ip4", "ip6"} {
		t.Run(phase, func(t *testing.T) {
			failure := errors.New("source unavailable")
			loader := mock.NewMockSources(gomock.NewController(t))
			server := netip.MustParseAddr("8.8.8.8")

			if phase == "download" {
				loader.EXPECT().Domains(gomock.Any(), "https://example.org/list").Return(nil, 0, failure)
			} else {
				loader.EXPECT().Domains(gomock.Any(), "https://example.org/list").
					Return(sources.DomainSet{"example.org": {}}, 0, nil)
				if phase == "ip4" {
					loader.EXPECT().Addresses(gomock.Any(), server, "example.org", "ip4").Return(nil, failure)
				} else {
					gomock.InOrder(
						loader.EXPECT().Addresses(gomock.Any(), server, "example.org", "ip4").
							Return([]netip.Prefix{netip.MustParsePrefix("93.184.215.14/32")}, nil),
						loader.EXPECT().Addresses(gomock.Any(), server, "example.org", "ip6").Return(nil, failure),
					)
				}
			}

			result, err := (Compiler{Sources: loader}).Compile(context.Background(), testConfig())
			if !errors.Is(err, failure) || result != nil {
				t.Fatalf("partial compile accepted: %+v, %v", result, err)
			}
		})
	}
}

func TestCompilerManualAndDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Settings.IPv6, cfg.Settings.DNS = "off", ""
	cfg.Services = []config.Service{{ID: "manual", Enabled: true, IPs: []string{"192.0.2.10", "192.0.2.0/24"}}}
	compiler := Compiler{Sources: mock.NewMockSources(gomock.NewController(t))}
	result, err := compiler.Compile(context.Background(), cfg)
	if err != nil || len(result.Plan.Targets) != 2 {
		t.Fatalf("manual compile: %+v, %v", result, err)
	}

	cfg.Settings.Enabled = false
	if result, err := compiler.Compile(context.Background(), cfg); err != nil || result != nil {
		t.Fatalf("disabled compile: %+v, %v", result, err)
	}
}

func TestCompilerValidation(t *testing.T) {
	for name, change := range map[string]func(*config.Config){
		"ipv6":     func(c *config.Config) { c.Settings.IPv6 = "" },
		"dns":      func(c *config.Config) { c.Settings.DNS = "" },
		"device":   func(c *config.Config) { c.Settings.Devices = []string{"bad device"} },
		"schedule": func(c *config.Config) { c.Settings.UpdateMinute = "60" },
		"empty":    func(c *config.Config) { c.Services = nil },
		"targets":  func(c *config.Config) { c.Services[0].Domains = nil },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig()
			cfg.Services = cfg.Services[:1]
			change(cfg)
			if _, err := (Compiler{Sources: mock.NewMockSources(gomock.NewController(t))}).Compile(context.Background(), cfg); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestPrefixes(t *testing.T) {
	for _, value := range []string{"0.0.0.0/0", "1.2.3.4/24", "fe80::1%eth0", "::ffff:1.2.3.4"} {
		if _, err := ParsePrefixes([]string{value}, true, false); err == nil {
			t.Errorf("accepted %q", value)
		}
	}

	if _, err := ParsePrefixes([]string{"2001:db8::1"}, false, false); err == nil {
		t.Error("accepted IPv6 with mode off")
	}

	if _, err := ParsePrefixes([]string{"192.0.2.0/24"}, false, true); err == nil {
		t.Error("accepted CIDR exception")
	}

	got, err := ParsePrefixes([]string{"192.0.2.10", "192.0.2.10"}, false, false)
	want := []netip.Prefix{netip.MustParsePrefix("192.0.2.10/32")}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("prefix normalization: %v, %v", got, err)
	}
}
