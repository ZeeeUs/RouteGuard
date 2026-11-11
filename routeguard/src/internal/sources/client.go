// Package sources reads bounded domain lists and resolves real destination addresses.
package sources

import (
	"context"
	"net"
	"net/http"
	"net/netip"
)

const (
	MaxDomains     = 256
	MaxSourceBytes = 128 * 1024
)

type Client struct {
	HTTP     *http.Client
	Resolver *net.Resolver
	server   netip.Addr
}

func (c *Client) Domains(ctx context.Context, url string) (DomainSet, int, error) {
	if c.HTTP == nil {
		c.HTTP = newHTTPClient()
	}

	return download(ctx, c.HTTP, url)
}

func (c *Client) Addresses(ctx context.Context, server netip.Addr, name, family string) ([]netip.Prefix, error) {
	if c.Resolver == nil || c.server.IsValid() && c.server != server {
		c.Resolver = ResolverFor(server)
		c.server = server
	}

	return resolve(ctx, c.Resolver, name, family)
}
