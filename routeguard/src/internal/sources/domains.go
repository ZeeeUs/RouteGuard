package sources

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type DomainSet map[string]struct{}

func DomainName(value string) (string, error) {
	name := strings.ToLower(strings.TrimSuffix(value, "."))
	labels := strings.Split(name, ".")
	if len(name) > 253 || len(labels) < 2 {
		return "", fmt.Errorf("invalid domain %q", value)
	}

	hasLetter := false

	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid domain %q", value)
		}

		hasLetter = false
		for _, ch := range label {
			switch {
			case ch >= 'a' && ch <= 'z':
				hasLetter = true
			case ch >= '0' && ch <= '9', ch == '-':
			default:
				return "", fmt.Errorf("invalid domain %q", value)
			}
		}
	}

	if !hasLetter {
		return "", fmt.Errorf("invalid domain suffix in %q", value)
	}

	return name, nil
}

func Parse(reader io.Reader) (DomainSet, int, error) {
	input := &io.LimitedReader{R: reader, N: MaxSourceBytes + 1}
	scanner := bufio.NewScanner(input)
	domains := make(DomainSet)
	skipped, line := 0, 0

	for scanner.Scan() {
		line++
		text := scanner.Text()
		if line == 1 {
			text = strings.TrimPrefix(text, "\ufeff")
		}

		record, _, _ := strings.Cut(text, "#")
		fields := strings.Fields(record)
		if len(fields) == 0 {
			continue
		}

		for _, attr := range fields[1:] {
			if !strings.HasPrefix(attr, "@") || len(attr) == 1 {
				return nil, 0, fmt.Errorf("malformed list record on line %d", line)
			}
		}

		kind, value, prefixed := strings.Cut(fields[0], ":")
		switch {
		case len(fields) > 1, prefixed && (kind == "include" || kind == "regexp" || kind == "keyword"):
			skipped++
			continue
		case !prefixed:
			value = kind
		case kind != "domain" && kind != "full":
			return nil, 0, fmt.Errorf("unsupported list record on line %d", line)
		}

		name, err := DomainName(value)
		if err != nil {
			return nil, 0, fmt.Errorf("list line %d: %w", line, err)
		}

		domains[name] = struct{}{}
		if len(domains) > MaxDomains {
			return nil, 0, fmt.Errorf("domain list exceeds %d names", MaxDomains)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	if input.N == 0 {
		return nil, 0, fmt.Errorf("domain list exceeds 128 KiB")
	}

	if len(domains) == 0 {
		return nil, 0, fmt.Errorf("list has no usable domain records")
	}

	return domains, skipped, nil
}
