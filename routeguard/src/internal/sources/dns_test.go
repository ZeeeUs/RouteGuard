package sources

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"
)

func dnsServer(t *testing.T, reply func([]byte) []byte) *net.Resolver {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buffer := make([]byte, 4096)
		for {
			count, peer, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			if response := reply(buffer[:count]); response != nil {
				_, _ = conn.WriteTo(response, peer)
			}
		}
	}()

	return &net.Resolver{PreferGo: true, StrictErrors: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, conn.LocalAddr().String())
	}}
}

func dnsReply(query []byte, code uint16, ip netip.Addr) []byte {
	end := 12
	for query[end] != 0 {
		end += int(query[end]) + 1
	}
	end += 5
	response := append([]byte(nil), query[:end]...)
	binary.BigEndian.PutUint16(response[2:4], 0x8180|code)
	binary.BigEndian.PutUint16(response[6:8], 0)
	binary.BigEndian.PutUint16(response[10:12], 0)
	if !ip.IsValid() {
		return response
	}

	binary.BigEndian.PutUint16(response[6:8], 1)
	kind := uint16(1)
	if ip.Is6() {
		kind = 28
	}
	answer := make([]byte, 12)
	binary.BigEndian.PutUint16(answer[0:2], 0xc00c)
	binary.BigEndian.PutUint16(answer[2:4], kind)
	binary.BigEndian.PutUint16(answer[4:6], 1)
	binary.BigEndian.PutUint32(answer[6:10], 60)
	binary.BigEndian.PutUint16(answer[10:12], uint16(len(ip.AsSlice())))
	response = append(response, answer...)
	return append(response, ip.AsSlice()...)
}

func TestResolveRealDNSProtocol(t *testing.T) {
	for _, family := range []string{"ip4", "ip6"} {
		t.Run(family, func(t *testing.T) {
			ip := netip.MustParseAddr("93.184.215.14")
			if family == "ip6" {
				ip = netip.MustParseAddr("2606:2800:220:1::1")
			}
			resolver := dnsServer(t, func(query []byte) []byte { return dnsReply(query, 0, ip) })
			result, err := resolve(context.Background(), resolver, "example.org", family)
			if err != nil || len(result) != 1 || result[0].Addr() != ip {
				t.Fatalf("resolve=%v,%v", result, err)
			}
		})
	}
}

func TestResolveNegativeAndFailureResponses(t *testing.T) {
	for _, code := range []uint16{0, 3, 2} {
		resolver := dnsServer(t, func(query []byte) []byte { return dnsReply(query, code, netip.Addr{}) })
		result, err := resolve(context.Background(), resolver, "example.org", "ip4")
		if code == 2 && err == nil {
			t.Fatal("SERVFAIL silently accepted")
		}
		if code != 2 && (err != nil || len(result) != 0) {
			t.Fatalf("negative answer=%v,%v", result, err)
		}
	}
}

func TestResolveRejectsSyntheticAndHonorsCancellation(t *testing.T) {
	resolver := dnsServer(t, func(query []byte) []byte { return dnsReply(query, 0, netip.MustParseAddr("198.18.0.10")) })
	if _, err := resolve(context.Background(), resolver, "example.org", "ip4"); err == nil {
		t.Fatal("accepted synthetic address")
	}

	silent := dnsServer(t, func([]byte) []byte { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := resolve(ctx, silent, "example.org", "ip4"); err == nil {
		t.Fatal("ignored DNS cancellation")
	}
}
