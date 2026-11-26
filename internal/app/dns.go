package app

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/miekg/dns"
)

// selfStub as a listener's stub answers with pier's own IP in the client's
// subnet, so each network gets the address it can reach pier on.
const selfStub = "self"

// stubFunc returns the IP to answer a client with; nil means no answer.
type stubFunc func(client net.IP) net.IP

// autoUpstream as a listener's upstream forwards to the nameservers in
// /etc/resolv.conf (in a container on a user-defined network: Docker's DNS).
const autoUpstream = "auto"

const resolvConf = "/etc/resolv.conf"

// serveDNS runs a DNS stub that answers every query in the given domains with
// the listener's stub IP: A for an IPv4 address, AAAA for an IPv6 address.
// With an upstream, other names are forwarded there, and the listener also
// serves TCP, which clients use to retry a truncated forwarded answer.
func serveDNS(l DNSListener, domains []string, verbose bool) error {
	stub := fixedStub(net.ParseIP(l.Stub))
	if l.Stub == selfStub {
		stub = ownIPStub(net.InterfaceAddrs)
	}

	mux := dns.NewServeMux()
	handler := makeStubHandler(stub, verbose)

	for _, d := range domains {
		mux.HandleFunc(d+".", handler)
	}

	nets := []string{"udp"}
	if len(l.Upstream) > 0 {
		servers, err := resolveUpstream(l.Upstream, resolvConf)
		if err != nil {
			return fmt.Errorf("DNS server error: %s: %w", l, err)
		}
		mux.HandleFunc(".", makeForwardHandler(servers, verbose))
		nets = append(nets, "tcp")
	}

	errCh := make(chan error, len(nets))
	for _, n := range nets {
		server := &dns.Server{Addr: l.Addr, Net: n, Handler: mux}
		go func() {
			errCh <- fmt.Errorf("DNS server error (%s): %w", n, server.ListenAndServe())
		}()
	}

	return <-errCh
}

// validateUpstream accepts "auto" alone, or IP[:port] addresses.
func validateUpstream(upstream []string) error {
	for _, u := range upstream {
		if u == autoUpstream {
			if len(upstream) > 1 {
				return fmt.Errorf("invalid --dns-listen upstream: %q cannot be combined with other servers", autoUpstream)
			}
			continue
		}
		host := u
		if h, _, err := net.SplitHostPort(u); err == nil {
			host = h
		}
		if net.ParseIP(host) == nil {
			return fmt.Errorf("invalid --dns-listen upstream %q: must be %q or IP[:port]", u, autoUpstream)
		}
	}
	return nil
}

// resolveUpstream returns host:port addresses; "auto" reads the nameservers
// from the resolv.conf file.
func resolveUpstream(upstream []string, conf string) ([]string, error) {
	if len(upstream) == 1 && upstream[0] == autoUpstream {
		cfg, err := dns.ClientConfigFromFile(conf)
		if err != nil {
			return nil, fmt.Errorf("upstream auto: %w", err)
		}
		if len(cfg.Servers) == 0 {
			return nil, fmt.Errorf("upstream auto: no nameserver in %s", conf)
		}
		upstream = cfg.Servers
	}

	servers := make([]string, len(upstream))
	for i, u := range upstream {
		if _, _, err := net.SplitHostPort(u); err != nil {
			u = net.JoinHostPort(u, "53")
		}
		servers[i] = u
	}
	return servers, nil
}

// makeForwardHandler relays a query to the servers in order, over the
// client's transport, and returns the first answer unchanged; SERVFAIL when
// none answers.
func makeForwardHandler(servers []string, verbose bool) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		name := ""
		if len(r.Question) > 0 {
			name = r.Question[0].Name
		}

		c := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
		if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
			c.Net = "tcp"
		}

		for _, srv := range servers {
			resp, _, err := c.Exchange(r, srv)
			if err != nil {
				log.Printf("[DNS] Forward: %s, upstream %s failed: %v", name, srv, err)
				continue
			}
			if verbose {
				log.Printf("[DNS] Forward: %s -> %s", name, srv)
			}
			w.WriteMsg(resp)
			return
		}

		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(m)
	}
}

func fixedStub(ip net.IP) stubFunc {
	return func(net.IP) net.IP { return ip }
}

// ownIPStub picks the own address whose subnet contains the client. The
// addresses are read on every query, so networks connected later are seen.
func ownIPStub(addrs func() ([]net.Addr, error)) stubFunc {
	return func(client net.IP) net.IP {
		if client == nil {
			return nil
		}
		list, err := addrs()
		if err != nil {
			return nil
		}
		for _, a := range list {
			if n, ok := a.(*net.IPNet); ok && n.Contains(client) {
				return n.IP
			}
		}
		return nil
	}
}

func makeStubHandler(stub stubFunc, verbose bool) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true

		client := clientIP(w.RemoteAddr())
		ip := stub(client)
		ip4 := ip.To4()

		for _, q := range r.Question {
			hdr := dns.RR_Header{Name: q.Name, Class: dns.ClassINET, Ttl: 60}

			switch {
			case ip == nil:
				if verbose {
					log.Printf("[DNS] Query: %s from %s -> no own IP in the client's subnet", q.Name, client)
				}
				continue
			case q.Qtype == dns.TypeA && ip4 != nil:
				hdr.Rrtype = dns.TypeA
				m.Answer = append(m.Answer, &dns.A{Hdr: hdr, A: ip4})
			case q.Qtype == dns.TypeAAAA && ip4 == nil:
				hdr.Rrtype = dns.TypeAAAA
				m.Answer = append(m.Answer, &dns.AAAA{Hdr: hdr, AAAA: ip})
			default:
				continue
			}

			if verbose {
				log.Printf("[DNS] Query: %s -> %s", q.Name, ip)
			}
		}

		w.WriteMsg(m)
	}
}

func clientIP(addr net.Addr) net.IP {
	switch a := addr.(type) {
	case *net.UDPAddr:
		return a.IP
	case *net.TCPAddr:
		return a.IP
	}
	return nil
}
