package nft

import (
	_ "embed"
	"encoding/json"
	"net/netip"
	"slices"
	"testing"

	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

// Captured from the isolated Linux integration test, not from a mock renderer.
//
//go:embed testdata/table.json
var kernelFixture []byte

func expectedSnapshot() *policy.Snapshot {
	return &policy.Snapshot{Plan: policy.Plan{
		Devices: []string{"wan", "wan2"},
		Targets: []netip.Prefix{
			netip.MustParsePrefix("10.2.0.0/30"), netip.MustParsePrefix("10.2.0.2/32"),
			netip.MustParsePrefix("10.3.0.2/32"), netip.MustParsePrefix("fd00:2::/126"),
			netip.MustParsePrefix("fd00:2::2/128"), netip.MustParsePrefix("fd00:3::2/128"),
		},
		Exempt: []netip.Prefix{netip.MustParsePrefix("10.2.0.3/32"), netip.MustParsePrefix("fd00:2::3/128")},
	}}
}

func TestRealKernelJSONMatchesExpectedProtection(t *testing.T) {
	health, err := Inspect(kernelFixture, expectedSnapshot())
	if err != nil || !health.Matches || len(health.Counters) != 4 {
		t.Fatalf("health=%+v,%v", health, err)
	}
	exists, err := TableExists(kernelFixture)
	if err != nil || !exists {
		t.Fatalf("table=%v,%v", exists, err)
	}
}

type document struct {
	Entries []map[string]map[string]any `json:"nftables"`
}

func changed(t *testing.T, change func(*document)) []byte {
	t.Helper()
	var doc document
	if err := json.Unmarshal(kernelFixture, &doc); err != nil {
		t.Fatal(err)
	}
	change(&doc)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDetectsPartialOrAlteredProtection(t *testing.T) {
	for name, change := range map[string]func(*document){
		"empty-table": func(d *document) { d.Entries = d.Entries[:2] },
		"missing-chain": func(d *document) {
			d.Entries = slices.DeleteFunc(d.Entries, func(e map[string]map[string]any) bool { return e["chain"]["name"] == "forward" })
		},
		"missing-rules": func(d *document) {
			d.Entries = slices.DeleteFunc(d.Entries, func(e map[string]map[string]any) bool { return e["rule"]["chain"] == "forward" })
		},
		"empty-set":          func(d *document) { d.Entries[4]["set"]["elem"] = []any{} },
		"different-priority": func(d *document) { d.Entries[2]["chain"]["prio"] = 0 },
		"dormant-table":      func(d *document) { d.Entries[1]["table"]["flags"] = []string{"dormant"} },
		"extra-rule":         func(d *document) { d.Entries = append(d.Entries, d.Entries[8]) },
		"early-accept":       func(d *document) { d.Entries[8]["rule"]["expr"].([]any)[2] = map[string]any{"accept": nil} },
	} {
		t.Run(name, func(t *testing.T) {
			health, err := Inspect(changed(t, change), expectedSnapshot())
			if err != nil || health.Matches || health.Reason == "" {
				t.Fatalf("drift accepted: %+v,%v", health, err)
			}
		})
	}
}

func TestCountersAndEquivalentIntervalsDoNotCauseReload(t *testing.T) {
	data := changed(t, func(d *document) {
		d.Entries[4]["set"]["elem"] = []any{map[string]any{"range": []string{"10.2.0.0", "10.2.0.3"}}, "10.3.0.2"}
		for _, entry := range d.Entries {
			if rule := entry["rule"]; rule != nil {
				for _, expression := range rule["expr"].([]any) {
					if counter, ok := expression.(map[string]any)["counter"]; ok {
						counter.(map[string]any)["packets"] = 1234
					}
				}
			}
		}
	})
	health, err := Inspect(data, expectedSnapshot())
	if err != nil || !health.Matches || health.Counters["forward4"].Packets != 1234 {
		t.Fatalf("health=%+v,%v", health, err)
	}
}

func TestLegacyRendererRoundTrip(t *testing.T) {
	expected := expectedSnapshot().Plan
	text := Render(expected)
	actual, err := DecodeLegacy(text)
	if err != nil || Render(actual) != text {
		t.Fatalf("legacy decode=%+v,%v", actual, err)
	}
	if _, err := DecodeLegacy(text + "table inet unrelated {}\n"); err == nil {
		t.Fatal("accepted arbitrary legacy nft text")
	}
}
