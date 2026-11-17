package sources

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestListEgressRequiresPublicAddressOrAnExactAllowlistEntry(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.1.1", "::1", "fd00::1", "64:ff9b::a00:1", "198.18.0.1", "::ffff:10.0.0.1"} {
		if publicListAddress(netip.MustParseAddr(address).Unmap()) {
			t.Errorf("allowed non-public address %s", address)
		}
	}
	for _, address := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if !publicListAddress(netip.MustParseAddr(address)) {
			t.Errorf("denied public address %s", address)
		}
	}

	path := filepath.Join(t.TempDir(), "private-list-hosts")
	if err := os.WriteFile(path, []byte("# approved list\nlists.example 10.0.0.2/32\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		host, address string
		want          bool
	}{
		{"Lists.Example.", "10.0.0.2", true},
		{"other.example", "10.0.0.2", false},
		{"lists.example", "10.0.0.3", false},
	} {
		allowed, err := allowedPrivateList(path, item.host, netip.MustParseAddr(item.address))
		if err != nil || allowed != item.want {
			t.Errorf("%s at %s: allowed=%t, err=%v", item.host, item.address, allowed, err)
		}
	}
}
