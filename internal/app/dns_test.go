package app

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// recorder captures the reply written by a DNS handler.
type recorder struct {
	dns.ResponseWriter
	remote net.Addr
	msg    *dns.Msg
}

func (r *recorder) RemoteAddr() net.Addr {
	if r.remote == nil {
		return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 40000}
	}
	return r.remote
}

func (r *recorder) WriteMsg(m *dns.Msg) error {
	r.msg = m
	return nil
}

func query(h dns.HandlerFunc, name string, qtype uint16) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), qtype)
	w := &recorder{}
	h(w, req)
	return w.msg
}

func TestStubHandler_IPv4(t *testing.T) {
	t.Parallel()

	h := makeStubHandler(fixedStub(net.ParseIP("127.0.0.1")), false)

	m := query(h, "app.lh", dns.TypeA)
	if len(m.Answer) != 1 || m.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("A answer = %v", m.Answer)
	}

	if m := query(h, "app.lh", dns.TypeAAAA); len(m.Answer) != 0 {
		t.Errorf("AAAA for IPv4 stub should be empty, got %v", m.Answer)
	}
}

func TestStubHandler_IPv6(t *testing.T) {
	t.Parallel()

	h := makeStubHandler(fixedStub(net.ParseIP("::1")), false)

	m := query(h, "app.lh", dns.TypeAAAA)
	if len(m.Answer) != 1 || m.Answer[0].(*dns.AAAA).AAAA.String() != "::1" {
		t.Errorf("AAAA answer = %v", m.Answer)
	}

	if m := query(h, "app.lh", dns.TypeA); len(m.Answer) != 0 {
		t.Errorf("A for IPv6 stub should be empty, got %v", m.Answer)
	}
}

func TestOwnIPStub(t *testing.T) {
	t.Parallel()

	stub := ownIPStub(func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			&net.IPNet{IP: net.ParseIP("172.17.0.5"), Mask: net.CIDRMask(16, 32)},
			&net.IPNet{IP: net.ParseIP("172.18.0.3"), Mask: net.CIDRMask(16, 32)},
			&net.IPNet{IP: net.ParseIP("fd00::5"), Mask: net.CIDRMask(64, 128)},
		}, nil
	})

	cases := map[string]string{
		"172.17.0.9": "172.17.0.5",
		"172.18.0.1": "172.18.0.3",
		"127.0.0.1":  "127.0.0.1",
		"fd00::9":    "fd00::5",
		"10.0.0.1":   "<nil>",
	}
	for client, want := range cases {
		if got := stub(net.ParseIP(client)).String(); got != want {
			t.Errorf("client %s: got %s, want %s", client, got, want)
		}
	}
	if got := stub(nil); got != nil {
		t.Errorf("nil client: got %s", got)
	}
}

func TestStubHandler_Self(t *testing.T) {
	t.Parallel()

	h := makeStubHandler(ownIPStub(func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("172.17.0.5"), Mask: net.CIDRMask(16, 32)}}, nil
	}), false)

	ask := func(client string) *dns.Msg {
		req := new(dns.Msg)
		req.SetQuestion("app.lh.", dns.TypeA)
		w := &recorder{remote: &net.UDPAddr{IP: net.ParseIP(client), Port: 40000}}
		h(w, req)
		return w.msg
	}

	if m := ask("172.17.0.9"); len(m.Answer) != 1 || m.Answer[0].(*dns.A).A.String() != "172.17.0.5" {
		t.Errorf("same subnet: answer = %v", m.Answer)
	}
	if m := ask("10.0.0.1"); m.Rcode != dns.RcodeSuccess || len(m.Answer) != 0 {
		t.Errorf("other subnet: rcode %d, answer %v", m.Rcode, m.Answer)
	}
}

func TestResolveUpstream(t *testing.T) {
	t.Parallel()

	conf := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(conf, []byte("nameserver 127.0.0.11\noptions ndots:0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveUpstream([]string{autoUpstream}, conf)
	if err != nil || !reflect.DeepEqual(got, []string{"127.0.0.11:53"}) {
		t.Errorf("auto: got %v, %v", got, err)
	}

	got, err = resolveUpstream([]string{"1.1.1.1", "8.8.8.8:5353", "::1"}, conf)
	if want := []string{"1.1.1.1:53", "8.8.8.8:5353", "[::1]:53"}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("servers: got %v, %v; want %v", got, err, want)
	}

	empty := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(empty, []byte("options ndots:0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveUpstream([]string{autoUpstream}, empty); err == nil {
		t.Error("auto without nameserver: expected error")
	}
}

// TestServeDNS_Forward runs a listener with an upstream: its domain is
// answered by the stub, other names are forwarded over UDP and TCP.
func TestServeDNS_Forward(t *testing.T) {
	t.Parallel()

	upstream := startDNS(t, dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		rr, _ := dns.NewRR(r.Question[0].Name + " 60 IN A 192.0.2.7")
		m.Answer = append(m.Answer, rr)
		w.WriteMsg(m)
	}))

	addr := freeAddr(t)
	go serveDNS(DNSListener{Addr: addr, Stub: "127.0.0.1", Upstream: []string{upstream}}, []string{"lh"}, false)

	for _, n := range []string{"udp", "tcp"} {
		c := &dns.Client{Net: n, Timeout: time.Second}
		cases := map[string]string{"app.lh.": "127.0.0.1", "example.com.": "192.0.2.7"}
		for name, want := range cases {
			var resp *dns.Msg
			var err error
			for range 50 { // wait for the listener
				m := new(dns.Msg)
				m.SetQuestion(name, dns.TypeA)
				if resp, _, err = c.Exchange(m, addr); err == nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err != nil || len(resp.Answer) != 1 || resp.Answer[0].(*dns.A).A.String() != want {
				t.Errorf("%s %s: got %v, %v; want %s", n, name, resp, err, want)
			}
		}
	}
}

func TestForwardHandler_AllFail(t *testing.T) {
	t.Parallel()

	h := makeForwardHandler([]string{freeAddr(t)}, false)
	if m := query(h, "example.com", dns.TypeA); m.Rcode != dns.RcodeServerFailure {
		t.Errorf("rcode = %d, want SERVFAIL", m.Rcode)
	}
}

// startDNS serves h on a free port, over UDP and TCP, and returns its address.
func startDNS(t *testing.T, h dns.Handler) string {
	t.Helper()

	addr := freeAddr(t)
	for _, n := range []string{"udp", "tcp"} {
		started := make(chan struct{})
		server := &dns.Server{Addr: addr, Net: n, Handler: h, NotifyStartedFunc: func() { close(started) }}
		go server.ListenAndServe()
		<-started
		t.Cleanup(func() { server.Shutdown() })
	}

	return addr
}

// freeAddr returns a 127.0.0.1 address with a port free for both UDP and TCP.
func freeAddr(t *testing.T) string {
	t.Helper()

	for range 20 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		if pc, err := net.ListenPacket("udp", addr); err == nil {
			pc.Close()
			return addr
		}
	}
	t.Fatal("no free port")
	return ""
}
