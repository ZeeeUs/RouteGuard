package platform

import (
	"context"
	"net"
	"net/netip"
	"syscall"
	"time"
)

// TCPProbe sends only a TCP handshake, without HTTP data or proxy settings.
type TCPProbe struct{}

func (TCPProbe) Connect(ctx context.Context, device string, address netip.Addr) error {
	dialer := net.Dialer{
		Timeout: 2 * time.Second,
		Control: func(_, _ string, raw syscall.RawConn) error {
			var bindErr error
			if err := raw.Control(func(fd uintptr) { bindErr = bindDevice(fd, device) }); err != nil {
				return err
			}
			return bindErr
		},
	}
	network := "tcp4"
	if address.Is6() {
		network = "tcp6"
	}
	conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), "443"))
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
