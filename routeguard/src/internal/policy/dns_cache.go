package policy

import (
	"net/netip"
	"slices"
	"time"
)

const negativeDNSGrace = 24 * time.Hour

// DNSCache uses a bit per domain for each address. Many domains can share
// answers without making the on-disk state scale as domains times addresses.
type DNSCache struct {
	Names        []string                `json:"names"`
	Addresses    map[netip.Prefix][]byte `json:"addresses"`
	MissingSince map[string]int64        `json:"missing_since,omitempty"`
}

func (c *DNSCache) remember(index int, prefix netip.Prefix) {
	mask := c.Addresses[prefix]
	if mask == nil {
		mask = make([]byte, (len(c.Names)+7)/8)
		c.Addresses[prefix] = mask
	}
	mask[index/8] |= 1 << (index % 8)
}

func (c *DNSCache) lastKnown(name, family string, now time.Time) ([]netip.Prefix, int64) {
	if c == nil {
		return nil, 0
	}
	index, exists := slices.BinarySearch(c.Names, name)
	if !exists {
		return nil, 0
	}
	since := c.MissingSince[family+" "+name]
	if since == 0 {
		since = now.Unix()
	}
	if now.Sub(time.Unix(since, 0)) >= negativeDNSGrace {
		return nil, 0
	}

	var result []netip.Prefix
	for prefix, mask := range c.Addresses {
		if prefix.Addr().Is4() == (family == "ip4") && index/8 < len(mask) && mask[index/8]&(1<<(index%8)) != 0 {
			result = append(result, prefix)
		}
	}

	return result, since
}
