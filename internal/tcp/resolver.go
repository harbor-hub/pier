package tcp

import (
	"context"
	"net"
	"time"
)

// NewResolver returns a resolver that sends every query to the DNS server at
// addr ("host[:port]", port 53 by default). An empty addr returns nil, which
// means the system resolver.
func NewResolver(addr string) *net.Resolver {
	if addr == "" {
		return nil
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 2 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
	}
}
