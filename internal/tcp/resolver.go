package tcp

import (
	"context"
	"net"
	"time"
)

// newResolver returns a resolver that sends every query to the DNS server at
// addr ("host[:port]", port 53 by default). An empty addr returns nil, which
// means the system resolver.
func newResolver(addr string) *net.Resolver {
	if addr == "" {
		return nil
	}
	addr = resolverAddr(addr)

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 2 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
	}
}

// resolverAddr adds the default DNS port to addr when it has none.
func resolverAddr(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return net.JoinHostPort(addr, "53")
	}
	return addr
}
