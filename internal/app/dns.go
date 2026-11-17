package app

import (
	"fmt"
	"log"
	"net"

	"github.com/miekg/dns"
)

// serveDNS runs a DNS stub that answers every query in the given domains with
// the stub IP: A for an IPv4 address, AAAA for an IPv6 address.
func serveDNS(addr string, domains []string, stub net.IP, verbose bool) error {
	mux := dns.NewServeMux()
	handler := makeStubHandler(stub, verbose)

	for _, d := range domains {
		mux.HandleFunc(d+".", handler)
	}

	server := &dns.Server{Addr: addr, Net: "udp", Handler: mux}
	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("DNS server error: %w", err)
	}

	return nil
}

func makeStubHandler(stub net.IP, verbose bool) dns.HandlerFunc {
	ip4 := stub.To4()

	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true

		for _, q := range r.Question {
			hdr := dns.RR_Header{Name: q.Name, Class: dns.ClassINET, Ttl: 60}

			switch {
			case q.Qtype == dns.TypeA && ip4 != nil:
				hdr.Rrtype = dns.TypeA
				m.Answer = append(m.Answer, &dns.A{Hdr: hdr, A: ip4})
			case q.Qtype == dns.TypeAAAA && ip4 == nil:
				hdr.Rrtype = dns.TypeAAAA
				m.Answer = append(m.Answer, &dns.AAAA{Hdr: hdr, AAAA: stub})
			default:
				continue
			}

			if verbose {
				log.Printf("[DNS] Query: %s -> %s", q.Name, stub)
			}
		}

		w.WriteMsg(m)
	}
}
