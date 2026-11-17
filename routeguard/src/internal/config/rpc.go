package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ParseRPC validates the same fields as uci export. A revision identifies the
// values and ordering rather than the transient IDs assigned to anonymous sections.
func ParseRPC(data []byte) (*Config, string, error) {
	type section struct {
		id        string
		kind      string
		index     int
		anonymous bool
		options   map[string]json.RawMessage
	}

	var reply struct {
		Values map[string]map[string]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return nil, "", err
	}

	sections := make([]section, 0, len(reply.Values))
	for id, values := range reply.Values {
		item := section{id: id, options: values}
		for key, dest := range map[string]any{".type": &item.kind, ".index": &item.index, ".anonymous": &item.anonymous} {
			if err := json.Unmarshal(values[key], dest); err != nil {
				return nil, "", fmt.Errorf("UCI section %s: invalid %s", id, key)
			}
		}
		sections = append(sections, item)
	}
	slices.SortFunc(sections, func(a, b section) int { return a.index - b.index })

	d := decoder{ids: make(map[string]bool)}
	anonymous := make([]bool, 0, len(sections))
	for _, item := range sections {
		if err := d.startSection([]string{"config", item.kind, item.id}); err != nil {
			return nil, "", err
		}
		if item.kind == "service" {
			anonymous = append(anonymous, item.anonymous)
		}

		for key, value := range item.options {
			if strings.HasPrefix(key, ".") {
				continue
			}
			if bytes.HasPrefix(value, []byte("[")) {
				var entries []string
				if err := json.Unmarshal(value, &entries); err != nil {
					return nil, "", err
				}
				for _, entry := range entries {
					if err := d.field("list", key, entry); err != nil {
						return nil, "", err
					}
				}
				continue
			}

			var option string
			if err := json.Unmarshal(value, &option); err != nil {
				return nil, "", err
			}
			if err := d.field("option", key, option); err != nil {
				return nil, "", err
			}
		}
	}
	if !d.settingsSeen {
		return nil, "", fmt.Errorf("expected a settings section named main")
	}

	revisionConfig := d.config
	revisionConfig.Services = slices.Clone(d.config.Services)
	for index, service := range revisionConfig.Services {
		if anonymous[index] {
			service.ID = fmt.Sprintf("@service[%d]", index)
			revisionConfig.Services[index] = service
		}
	}
	encoded, err := json.Marshal(revisionConfig)
	if err != nil {
		return nil, "", err
	}
	revision := sha256.Sum256(encoded)
	return &d.config, hex.EncodeToString(revision[:]), nil
}
