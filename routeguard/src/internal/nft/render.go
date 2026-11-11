// Package nft renders and verifies the exclusively owned inet routeguard table.
package nft

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

const Destroy = "destroy table inet routeguard\n"

type addressSet struct {
	name   string
	kind   string
	values []netip.Prefix
}

type rule struct {
	family  string
	set     string
	verdict string
	comment string
}

func sets(plan policy.Plan) []addressSet {
	var result []addressSet
	for _, group := range []struct {
		name     string
		prefixes []netip.Prefix
	}{{"blocked", plan.Targets}, {"exempt", plan.Exempt}} {
		for _, v4 := range []bool{true, false} {
			set := addressSet{name: group.name + "6", kind: "ipv6_addr"}
			if v4 {
				set.name, set.kind = group.name+"4", "ipv4_addr"
			}

			for _, prefix := range group.prefixes {
				if prefix.Addr().Is4() == v4 {
					set.values = append(set.values, prefix)
				}
			}

			if len(set.values) > 0 {
				set.values = policy.SortPrefixes(set.values)
				result = append(result, set)
			}
		}
	}

	return result
}

func rules(groups []addressSet, hook string) []rule {
	has := make(map[string]bool, len(groups))
	for _, set := range groups {
		has[set.name] = true
	}

	var result []rule
	for _, version := range []string{"4", "6"} {
		if !has["blocked"+version] {
			continue
		}

		family := "ip"
		if version == "6" {
			family = "ip6"
		}

		if has["exempt"+version] {
			result = append(result, rule{family: family, set: "exempt" + version, verdict: "return"})
		}

		result = append(result, rule{family: family, set: "blocked" + version, verdict: "drop", comment: hook + version})
	}

	return result
}

func Render(plan policy.Plan) string {
	var output strings.Builder
	output.WriteString("table inet routeguard {\n")
	groups := sets(plan)

	for _, set := range groups {
		fmt.Fprintf(&output, " set %s { type %s; flags interval; auto-merge; elements = { ", set.name, set.kind)
		for index, prefix := range set.values {
			if index != 0 {
				output.WriteString(", ")
			}

			output.WriteString(prefix.String())
		}

		output.WriteString(" }; }\n")
	}

	devices := make([]string, 0, len(plan.Devices))
	for _, device := range plan.Devices {
		devices = append(devices, strconv.Quote(device))
	}

	for _, hook := range []string{"forward", "output"} {
		fmt.Fprintf(&output, " chain %s { type filter hook %s priority -1; policy accept;\n", hook, hook)
		for _, rule := range rules(groups, hook) {
			fmt.Fprintf(&output, "  oifname { %s } %s daddr @%s ", strings.Join(devices, ", "), rule.family, rule.set)
			if rule.verdict == "drop" {
				fmt.Fprintf(&output, "counter drop comment %q\n", rule.comment)
			} else {
				output.WriteString("return\n")
			}
		}

		output.WriteString(" }\n")
	}

	output.WriteString("}\n")
	return output.String()
}

func Batch(snapshot *policy.Snapshot) string {
	if snapshot == nil {
		return Destroy
	}

	return Destroy + Render(snapshot.Plan)
}
