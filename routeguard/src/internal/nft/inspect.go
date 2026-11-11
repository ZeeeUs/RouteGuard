package nft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ZeeeUs/RouteGuard/internal/policy"
)

type Counter struct {
	Packets uint64 `json:"packets"`
	Bytes   uint64 `json:"bytes"`
}

type Health struct {
	Present  bool               `json:"present"`
	Matches  bool               `json:"matches"`
	Reason   string             `json:"reason,omitempty"`
	Counters map[string]Counter `json:"counters"`
}

type object struct {
	Family   string            `json:"family"`
	Table    string            `json:"table"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Flags    []string          `json:"flags"`
	Hook     string            `json:"hook"`
	Priority int               `json:"prio"`
	Policy   string            `json:"policy"`
	Chain    string            `json:"chain"`
	Comment  string            `json:"comment"`
	Elements []json.RawMessage `json:"elem"`
	Expr     []json.RawMessage `json:"expr"`
	Timeout  json.RawMessage   `json:"timeout"`
}

func TableExists(data []byte) (bool, error) {
	entries, err := readEntries(data)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if raw, ok := entry["table"]; ok {
			var table object
			if err := json.Unmarshal(raw, &table); err != nil {
				return false, err
			}

			if table.Family == "inet" && table.Name == "routeguard" {
				return true, nil
			}
		}
	}

	return false, nil
}

func readEntries(data []byte) ([]map[string]json.RawMessage, error) {
	var document struct {
		Entries []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}

	if document.Entries == nil {
		return nil, fmt.Errorf("missing nftables JSON envelope")
	}

	return document.Entries, nil
}

func Inspect(data []byte, expected *policy.Snapshot) (Health, error) {
	health := Health{Present: true, Counters: make(map[string]Counter)}
	entries, err := readEntries(data)
	if err != nil {
		return health, err
	}

	if expected == nil {
		health.Reason = "unexpected RouteGuard table while protection is disabled"
		return health, nil
	}

	if err := verify(entries, expected.Plan, health.Counters); err != nil {
		health.Reason = err.Error()
		return health, nil
	}

	health.Matches = true
	return health, nil
}

func verify(entries []map[string]json.RawMessage, plan policy.Plan, counters map[string]Counter) error {
	expectedSets := make(map[string]addressSet)
	for _, set := range sets(plan) {
		expectedSets[set.name] = set
	}

	chains := make(map[string]bool)
	chainRules := make(map[string][]object)
	tables := 0
	for _, entry := range entries {
		if len(entry) != 1 {
			return fmt.Errorf("unexpected nftables object")
		}

		for kind, raw := range entry {
			if kind == "metainfo" {
				continue
			}

			var item object
			if err := json.Unmarshal(raw, &item); err != nil {
				return err
			}

			if item.Family != "inet" || kind != "table" && item.Table != "routeguard" {
				return fmt.Errorf("unexpected table identity")
			}

			switch kind {
			case "table":
				tables++
				if item.Name != "routeguard" || len(item.Flags) != 0 {
					return fmt.Errorf("unexpected table name or flags")
				}
			case "set":
				set, ok := expectedSets[item.Name]
				if !ok || !setMatches(item, set) {
					return fmt.Errorf("address set %s differs from the saved rules", item.Name)
				}

				delete(expectedSets, item.Name)
			case "chain":
				if !chainMatches(item) || chains[item.Name] {
					return fmt.Errorf("chain %s differs from the saved rules", item.Name)
				}

				chains[item.Name] = true
			case "rule":
				chainRules[item.Chain] = append(chainRules[item.Chain], item)
			default:
				return fmt.Errorf("unexpected nftables object %s", kind)
			}
		}
	}

	if tables != 1 || len(expectedSets) != 0 || len(chains) != 2 || len(chainRules) != 2 {
		return fmt.Errorf("missing table, sets, chains or rules")
	}

	return verifyRules(plan, chainRules, counters)
}

func chainMatches(item object) bool {
	return (item.Name == "forward" || item.Name == "output") && item.Hook == item.Name &&
		item.Type == "filter" && item.Priority == -1 && item.Policy == "accept" && len(item.Flags) == 0
}

func setMatches(item object, expected addressSet) bool {
	if item.Type != expected.kind || !slices.Equal(item.Flags, []string{"interval"}) || len(item.Timeout) != 0 {
		return false
	}

	actual := make([]interval, 0, len(item.Elements))
	for _, raw := range item.Elements {
		value, err := readElement(raw)
		if err != nil {
			return false
		}

		actual = append(actual, value)
	}

	want := make([]interval, 0, len(expected.values))
	for _, prefix := range expected.values {
		want = append(want, prefixInterval(prefix))
	}

	return slices.Equal(normalize(actual), normalize(want))
}

func verifyRules(plan policy.Plan, actual map[string][]object, counters map[string]Counter) error {
	groups := sets(plan)
	for _, hook := range []string{"forward", "output"} {
		expected := rules(groups, hook)
		if len(actual[hook]) != len(expected) {
			return fmt.Errorf("unexpected rule count in %s", hook)
		}

		for index, want := range expected {
			value, err := verifyRule(actual[hook][index], want, plan.Devices)
			if err != nil {
				return fmt.Errorf("%s rule %d: %w", hook, index, err)
			}

			if want.comment != "" {
				counters[want.comment] = value
			}
		}
	}

	return nil
}

func verifyRule(item object, want rule, devices []string) (Counter, error) {
	length := 3
	if want.verdict == "drop" {
		length = 4
	}

	if len(item.Expr) != length || item.Comment != want.comment ||
		!deviceMatch(item.Expr[0], devices) || !destinationMatch(item.Expr[1], want) ||
		!jsonEqual(item.Expr[length-1], map[string]any{want.verdict: nil}) {
		return Counter{}, fmt.Errorf("match, verdict or order differs from saved rules")
	}

	var counter struct {
		Value *Counter `json:"counter"`
	}
	if want.verdict == "drop" {
		if err := json.Unmarshal(item.Expr[2], &counter); err != nil || counter.Value == nil {
			return Counter{}, fmt.Errorf("missing counter")
		}

		return *counter.Value, nil
	}

	return Counter{}, nil
}

func deviceMatch(raw json.RawMessage, devices []string) bool {
	var expression struct {
		Match struct {
			Op    string          `json:"op"`
			Left  json.RawMessage `json:"left"`
			Right json.RawMessage `json:"right"`
		} `json:"match"`
	}
	if json.Unmarshal(raw, &expression) != nil || expression.Match.Op != "==" ||
		!jsonEqual(expression.Match.Left, map[string]any{"meta": map[string]string{"key": "oifname"}}) {
		return false
	}

	var single string
	var names struct {
		Set []string `json:"set"`
	}
	if json.Unmarshal(expression.Match.Right, &single) == nil {
		names.Set = []string{single}
	} else if json.Unmarshal(expression.Match.Right, &names) != nil {
		return false
	}

	slices.Sort(names.Set)
	want := slices.Clone(devices)
	slices.Sort(want)
	return slices.Equal(names.Set, want)
}

func destinationMatch(raw json.RawMessage, want rule) bool {
	return jsonEqual(raw, map[string]any{"match": map[string]any{
		"op": "==", "left": map[string]any{"payload": map[string]string{"protocol": want.family, "field": "daddr"}},
		"right": "@" + want.set,
	}})
}

func jsonEqual(raw json.RawMessage, expected any) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}

	actual, _ := json.Marshal(value)
	want, _ := json.Marshal(expected)
	return bytes.Equal(actual, want)
}
