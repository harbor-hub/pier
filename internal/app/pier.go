// Package app implements pier: an HTTP/HTTPS reverse proxy with a DNS stub,
// for backends the client cannot reach directly (e.g. container IPs on Docker
// Desktop or in WSL2 seen from Windows).
//
//	client -> my-app.<domain> -> --dns-stub IP (DNS stub)
//	       -> pier (--http-listen / --https-listen)
//	       -> backend target looked up by hostname in the records
//
// pier must be able to reach the backend targets (same host/network).
package app

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const minSourceRefresh = 500 * time.Millisecond

type Pier struct {
	Version       string
	DNSAddr       string
	DNSDomains    []string
	DNSStub       string
	HTTPAddr      string
	HTTPSAddr     string
	HTTPSCert     string
	HTTPSKey      string
	Source        string
	SourceToken   string
	SourceRefresh time.Duration
	Verbose       bool
}

func New() *Pier {
	return &Pier{}
}

func (p *Pier) Run() error {
	if err := p.validate(); err != nil {
		return err
	}

	fmt.Printf("\nPier %s\n\n  dns-listen:     %s\n  dns-domain:     %s\n  dns-stub:       %s\n  http-listen:    %s\n  https-listen:   %s\n  https-cert:     %s\n  source:         %s\n  source-refresh: %s\n\n",
		p.Version,
		orDisabled(p.DNSAddr),
		strings.Join(p.DNSDomains, ", "),
		p.DNSStub,
		orDisabled(p.HTTPAddr),
		orDisabled(p.HTTPSAddr),
		orDash(p.HTTPSCert),
		orDash(p.Source),
		p.SourceRefresh,
	)

	if (p.HTTPAddr != "" || p.HTTPSAddr != "") && p.Source == "" {
		log.Printf("[SOURCE] no --source given: every request returns 404")
	}

	store := NewStore(p.Source, p.SourceToken)
	store.Refresh()
	go store.Run(p.SourceRefresh)

	httpsPort := ""
	if p.HTTPSAddr != "" {
		_, httpsPort, _ = net.SplitHostPort(p.HTTPSAddr)
	}
	handler := newProxyHandler(store, httpsPort, p.Verbose)

	errCh := make(chan error, 3)

	if p.DNSAddr != "" {
		go func() {
			errCh <- serveDNS(p.DNSAddr, p.DNSDomains, net.ParseIP(p.DNSStub), p.Verbose)
		}()
	}

	if p.HTTPSAddr != "" {
		var cert *tls.Certificate
		if p.HTTPSCert != "" {
			c, err := tls.LoadX509KeyPair(p.HTTPSCert, p.HTTPSKey)
			if err != nil {
				return fmt.Errorf("--https-cert / --https-key: %w", err)
			}
			cert = &c
		}

		ln, err := net.Listen("tcp", p.HTTPSAddr)
		if err != nil {
			return fmt.Errorf("HTTPS server error: %w", err)
		}

		server := newHTTPSServer(p.HTTPSAddr, store, handler, cert, p.Verbose)
		go func() {
			errCh <- fmt.Errorf("HTTPS server error: %w", server.Serve(ln))
		}()
	}

	if p.HTTPAddr != "" {
		server := newHTTPServer(p.HTTPAddr, handler)
		go func() {
			errCh <- fmt.Errorf("HTTP server error: %w", server.ListenAndServe())
		}()
	}

	return <-errCh
}

// newHTTPServer limits how long a client may take to send headers and keep an
// idle connection. There is no read/write timeout for the whole request:
// proxied uploads, downloads, SSE and WebSockets may legitimately run long.
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func (p *Pier) validate() error {
	if p.DNSAddr == "" && p.HTTPAddr == "" && p.HTTPSAddr == "" {
		return errors.New("nothing to do: set --dns-listen, --http-listen and/or --https-listen")
	}
	if p.DNSAddr != "" {
		if len(p.DNSDomains) == 0 {
			return errors.New("--dns-listen needs --dns-domain")
		}
		if net.ParseIP(p.DNSStub) == nil {
			return fmt.Errorf("invalid --dns-stub %q: must be an IP address", p.DNSStub)
		}
	}
	if (p.HTTPSCert == "") != (p.HTTPSKey == "") {
		return errors.New("--https-cert and --https-key must be set together")
	}
	if p.HTTPSAddr != "" {
		if _, _, err := net.SplitHostPort(p.HTTPSAddr); err != nil {
			return fmt.Errorf("invalid --https-listen: %w", err)
		}
	}
	if p.SourceRefresh < minSourceRefresh {
		return fmt.Errorf("--source-refresh must be at least %s", minSourceRefresh)
	}

	return nil
}

func orDisabled(s string) string {
	if s == "" {
		return "(disabled)"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
