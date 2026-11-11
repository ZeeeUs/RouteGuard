package nft

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

// DecodeLegacy accepts only the exact grammar emitted by the prototype renderer.
// It never executes saved text while migrating it to a structured plan.
func DecodeLegacy(text string) (policy.Plan, error) {
	var plan policy.Plan
	setPattern := regexp.MustCompile(`(?m)^ set (blocked[46]|exempt[46]) \{ type (ipv[46]_addr); flags interval; auto-merge; elements = \{ ([^{}]+) \}; \}$`)
	devicePattern := regexp.MustCompile(`oifname \{ ([^{}]+) \}`)

	for _, match := range setPattern.FindAllStringSubmatch(text, -1) {
		values := strings.Split(match[3], ", ")
		exempt := strings.HasPrefix(match[1], "exempt")
		prefixes, err := policy.ParsePrefixes(values, true, exempt)
		if err != nil {
			return plan, err
		}

		if exempt {
			plan.Exempt = append(plan.Exempt, prefixes...)
		} else {
			plan.Targets = append(plan.Targets, prefixes...)
		}
	}

	match := devicePattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return plan, fmt.Errorf("legacy rules have no WAN selector")
	}

	for _, quoted := range strings.Split(match[1], ", ") {
		device, err := strconv.Unquote(quoted)
		if err != nil {
			return plan, err
		}

		plan.Devices = append(plan.Devices, device)
	}

	if err := policy.ValidateDevices(plan.Devices); err != nil {
		return plan, err
	}

	if strings.TrimSpace(Render(plan)) != strings.TrimSpace(text) {
		return plan, fmt.Errorf("unrecognized legacy rules; keep the snapshot and reapply explicit UCI settings")
	}

	return plan, nil
}
