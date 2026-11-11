package nft

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
)

type interval struct {
	first netip.Addr
	last  netip.Addr
}

func prefixInterval(prefix netip.Prefix) interval {
	first := prefix.Masked().Addr()
	bytes := first.As16()
	start := prefix.Bits()
	if first.Is4() {
		start += 96
	}

	for bit := start; bit < 128; bit++ {
		bytes[bit/8] |= 1 << (7 - bit%8)
	}

	last := netip.AddrFrom16(bytes)
	if first.Is4() {
		last = last.Unmap()
	}

	return interval{first, last}
}

// nft auto-merges overlapping and adjacent intervals. Compare their union,
// rather than the textual CIDR spelling or element count.
func normalize(values []interval) []interval {
	slices.SortFunc(values, func(a, b interval) int {
		if order := a.first.Compare(b.first); order != 0 {
			return order
		}

		return a.last.Compare(b.last)
	})

	result := values[:0]
	for _, value := range values {
		if len(result) > 0 {
			previous := &result[len(result)-1]
			if previous.first.BitLen() == value.first.BitLen() &&
				(value.first.Compare(previous.last) <= 0 || value.first == previous.last.Next()) {
				if value.last.Compare(previous.last) > 0 {
					previous.last = value.last
				}

				continue
			}
		}

		result = append(result, value)
	}

	return result
}

func readElement(raw json.RawMessage) (interval, error) {
	var address string
	if json.Unmarshal(raw, &address) == nil {
		ip, err := netip.ParseAddr(address)
		return interval{ip, ip}, err
	}

	var element struct {
		Prefix *struct {
			Address string `json:"addr"`
			Length  int    `json:"len"`
		} `json:"prefix"`
		Range []string `json:"range"`
	}
	if err := json.Unmarshal(raw, &element); err != nil {
		return interval{}, err
	}

	if element.Prefix != nil {
		ip, err := netip.ParseAddr(element.Prefix.Address)
		prefix := netip.PrefixFrom(ip, element.Prefix.Length)
		if err != nil || !prefix.IsValid() {
			return interval{}, fmt.Errorf("invalid prefix element")
		}

		return prefixInterval(prefix), nil
	}

	if len(element.Range) == 2 {
		first, firstErr := netip.ParseAddr(element.Range[0])
		last, lastErr := netip.ParseAddr(element.Range[1])
		if firstErr == nil && lastErr == nil && first.BitLen() == last.BitLen() && first.Compare(last) <= 0 {
			return interval{first, last}, nil
		}
	}

	return interval{}, fmt.Errorf("unsupported set element")
}
