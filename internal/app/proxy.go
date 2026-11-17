package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

type targetKey struct{}

// proxyHandler routes a request by its Host header to the upstream chosen
// from the matching record.
type proxyHandler struct {
	store     *Store
	httpsPort string // empty: HTTPS disabled
	verbose   bool

	proxy         *httputil.ReverseProxy
	insecureProxy *httputil.ReverseProxy

	warned sync.Map
}

// newProxyHandler returns the handler; httpsPort is pier's HTTPS port, or
// empty when HTTPS is disabled (redirects are then not possible).
func newProxyHandler(store *Store, httpsPort string, verbose bool) *proxyHandler {
	insecure := http.DefaultTransport.(*http.Transport).Clone()
	insecure.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return &proxyHandler{
		store:         store,
		httpsPort:     httpsPort,
		verbose:       verbose,
		proxy:         newReverseProxy(http.DefaultTransport),
		insecureProxy: newReverseProxy(insecure),
	}
}

func newReverseProxy(transport http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(pr.In.Context().Value(targetKey{}).(*url.URL))
			pr.Out.Host = pr.In.Host
			pr.SetXForwarded()
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[HTTP] %s: %v", r.Host, err)
			http.Error(w, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		},
	}
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := targetHost(r.Host)

	clientScheme := "http"
	if r.TLS != nil {
		clientScheme = "https"
	}

	rec, ok := h.store.Lookup(host)
	if !ok {
		h.notFound(w, host)
		return
	}

	if clientScheme == "http" && rec.Labels[labelRedirect] == "https" {
		if h.httpsPort != "" {
			h.redirect(w, r, host)
			return
		}
		if _, dup := h.warned.LoadOrStore(host, true); !dup {
			log.Printf("[HTTP] %s: %s=https ignored, HTTPS is disabled", host, labelRedirect)
		}
	}

	up, err := selectUpstream(rec.Labels, clientScheme)
	if errors.Is(err, errNoUpstream) {
		http.Error(w, fmt.Sprintf("404 Not Found: host %s has no upstream for %s", host, strings.ToUpper(clientScheme)), http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("[HTTP] %s: %v", host, err)
		http.Error(w, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		return
	}

	if up.Scheme == "tls" {
		http.Error(w, fmt.Sprintf("421 Misdirected Request: host %s uses TLS passthrough; connect with HTTPS (SNI %s)", host, host), http.StatusMisdirectedRequest)
		return
	}

	targetHost := up.Host
	if targetHost == "" {
		targetHost = rec.Target
	}
	target := &url.URL{Scheme: up.Scheme, Host: net.JoinHostPort(targetHost, up.Port)}

	if h.verbose {
		log.Printf("[HTTP] %s %s%s -> %s", r.Method, host, r.URL.RequestURI(), target)
	}

	proxy := h.proxy
	if up.Insecure {
		proxy = h.insecureProxy
	}
	proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}

func (h *proxyHandler) redirect(w http.ResponseWriter, r *http.Request, host string) {
	if h.httpsPort != "443" {
		host = net.JoinHostPort(host, h.httpsPort)
	}
	http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
}

func (h *proxyHandler) notFound(w http.ResponseWriter, host string) {
	var b strings.Builder
	fmt.Fprintf(&b, "404 Not Found: unknown host %s\n", host)

	if hosts := h.store.Hosts(); len(hosts) > 0 {
		b.WriteString("\nKnown hosts:\n")
		for _, known := range hosts {
			fmt.Fprintf(&b, "  %s\n", known)
		}
	}

	http.Error(w, b.String(), http.StatusNotFound)
}

// targetHost strips the port from the Host header and normalizes it.
func targetHost(hostHeader string) string {
	host, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		host = hostHeader
	}

	return normalizeHost(host)
}
