package tcp

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// dnsServer answers A queries for any name with the IP set on it, or NXDOMAIN
// when none is set.
type dnsServer struct {
	mu sync.Mutex
	ip net.IP
}

func (s *dnsServer) set(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ip = net.ParseIP(ip)
}

func (s *dnsServer) start(t *testing.T) string {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		s.mu.Lock()
		ip := s.ip
		s.mu.Unlock()
		if ip == nil {
			m.Rcode = dns.RcodeNameError
		}
		for _, q := range r.Question {
			if q.Qtype == dns.TypeA && ip != nil {
				m.Answer = append(m.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
					A:   ip,
				})
			}
		}
		w.WriteMsg(m)
	})}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })

	return pc.LocalAddr().String()
}

func TestNewResolver_SystemWhenEmpty(t *testing.T) {
	t.Parallel()

	if newResolver("") != nil {
		t.Error("empty address should mean the system resolver (nil)")
	}
}

// The target name is resolved on every connection: when the DNS answer
// changes (a container recreated with a new IP), the next connection goes to
// the new address.
func TestProxy_ResolvesOnEveryConnection(t *testing.T) {
	t.Parallel()

	// Two backends on the same port, on two loopback addresses.
	first := backend(t, "127.0.0.1:0", "first")
	_, port, _ := net.SplitHostPort(first)
	backend(t, net.JoinHostPort("127.0.0.2", port), "second")

	dnsSrv := &dnsServer{}
	dnsSrv.set("127.0.0.1")
	dnsAddr := dnsSrv.start(t)

	p := &Proxy{
		Mappings:    []Mapping{{Listen: "127.0.0.1:0", Target: net.JoinHostPort("db.test", port)}},
		Resolver:    dnsAddr,
		DialTimeout: 2 * time.Second,
	}
	startProxy(t, p)
	addr := p.Addrs()[0].String()

	if reply, err := roundTrip(t, addr, "x"); err != nil || reply != "first:x" {
		t.Fatalf("got %q, %v; want first:x", reply, err)
	}

	dnsSrv.set("127.0.0.2")
	if reply, err := roundTrip(t, addr, "x"); err != nil || reply != "second:x" {
		t.Errorf("after the IP change: got %q, %v; want second:x", reply, err)
	}
}

func TestNewResolver_DefaultPort53(t *testing.T) {
	t.Parallel()

	// No server on port 53 here: only check that the port is added and used.
	r := newResolver("127.0.0.1")
	conn, err := r.Dial(t.Context(), "udp", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := conn.RemoteAddr().String(); got != "127.0.0.1:53" {
		t.Errorf("dials %s, want 127.0.0.1:53", got)
	}
}

// Go names the server from /etc/resolv.conf in lookup errors; the log must
// name the configured resolver, where the query actually went.
func TestProxy_LookupErrorNamesResolver(t *testing.T) {
	t.Parallel()

	dnsAddr := (&dnsServer{}).start(t) // no IP: every name is NXDOMAIN

	l := &logs{}
	p := &Proxy{
		Mappings:    []Mapping{{Listen: "127.0.0.1:0", Target: "unknown.test:3306"}},
		Resolver:    dnsAddr,
		DialTimeout: 2 * time.Second,
		Logf:        l.logf,
	}
	startProxy(t, p)

	if reply, _ := roundTrip(t, p.Addrs()[0].String(), "x"); reply != "" {
		t.Errorf("got %q, want the connection closed", reply)
	}
	if want := "lookup unknown.test on " + dnsAddr + ": no such host"; !strings.Contains(l.String(), want) {
		t.Errorf("log = %q, want it to contain %q", l, want)
	}
}
