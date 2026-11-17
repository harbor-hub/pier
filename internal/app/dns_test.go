package app

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

// recorder captures the reply written by a DNS handler.
type recorder struct {
	dns.ResponseWriter
	msg *dns.Msg
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

	h := makeStubHandler(net.ParseIP("127.0.0.1"), false)

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

	h := makeStubHandler(net.ParseIP("::1"), false)

	m := query(h, "app.lh", dns.TypeAAAA)
	if len(m.Answer) != 1 || m.Answer[0].(*dns.AAAA).AAAA.String() != "::1" {
		t.Errorf("AAAA answer = %v", m.Answer)
	}

	if m := query(h, "app.lh", dns.TypeA); len(m.Answer) != 0 {
		t.Errorf("A for IPv6 stub should be empty, got %v", m.Answer)
	}
}
