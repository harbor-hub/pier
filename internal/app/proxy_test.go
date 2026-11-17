package app

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// backend starts a test server that echoes the Host header, the forwarded
// proto and the request URI.
func backend(t *testing.T, tlsServer bool) (host, port string) {
	t.Helper()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Host+" "+r.Header.Get("X-Forwarded-Proto")+" "+r.URL.RequestURI())
	})

	var srv *httptest.Server
	if tlsServer {
		srv = httptest.NewTLSServer(h)
	} else {
		srv = httptest.NewServer(h)
	}
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	host, port, _ = net.SplitHostPort(u.Host)

	return host, port
}

func newTestHandler(records []Record, httpsPort string) *proxyHandler {
	s := newTestStore(&fakeSource{records: records})
	s.Refresh()
	return newProxyHandler(s, httpsPort, false)
}

func serve(h http.Handler, target string, isTLS bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if isTLS {
		r.TLS = &tls.ConnectionState{}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestProxy_HTTPUpstream(t *testing.T) {
	t.Parallel()

	ip, port := backend(t, false)
	h := newTestHandler([]Record{{Host: "app.lh", Target: ip, Labels: map[string]string{"pier.upstream": port}}}, "")

	w := serve(h, "http://App.LH:8080/x?y=1", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if got, want := w.Body.String(), "App.LH:8080 http /x?y=1"; got != want {
		t.Errorf("body = %q, want %q (Host header must be preserved)", got, want)
	}
}

func TestProxy_HTTPSClientSetsForwardedProto(t *testing.T) {
	t.Parallel()

	ip, port := backend(t, false)
	h := newTestHandler([]Record{{Host: "app.lh", Target: ip, Labels: map[string]string{"pier.upstream": port}}}, "443")

	w := serve(h, "https://app.lh/", true)
	if !strings.Contains(w.Body.String(), " https ") {
		t.Errorf("body = %q, want X-Forwarded-Proto https", w.Body)
	}
}

func TestProxy_TLSBackend(t *testing.T) {
	t.Parallel()

	ip, port := backend(t, true)

	insecure := newTestHandler([]Record{{Host: "app.lh", Target: ip, Labels: map[string]string{"pier.upstream": "https://:" + port + "?insecure=true"}}}, "")
	if w := serve(insecure, "http://app.lh/", false); w.Code != http.StatusOK {
		t.Errorf("insecure: status %d: %s", w.Code, w.Body)
	}

	verified := newTestHandler([]Record{{Host: "app.lh", Target: ip, Labels: map[string]string{"pier.upstream": "https://:" + port}}}, "")
	if w := serve(verified, "http://app.lh/", false); w.Code != http.StatusBadGateway {
		t.Errorf("verified self-signed: status %d, want 502", w.Code)
	}
}

func TestProxy_UnknownHost(t *testing.T) {
	t.Parallel()

	h := newTestHandler([]Record{{Host: "known.lh", Target: "10.0.0.1"}}, "")

	w := serve(h, "http://nope.lh/", false)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "known.lh") {
		t.Errorf("body should list known hosts: %q", w.Body)
	}
}

func TestProxy_NoUpstreamForScheme(t *testing.T) {
	t.Parallel()

	h := newTestHandler([]Record{{Host: "app.lh", Target: "10.0.0.1", Labels: map[string]string{"pier.upstream.https": "3000"}}}, "")

	w := serve(h, "http://app.lh/", false)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "no upstream for HTTP") {
		t.Errorf("status %d, body %q", w.Code, w.Body)
	}
}

func TestProxy_InvalidUpstream(t *testing.T) {
	t.Parallel()

	h := newTestHandler([]Record{{Host: "app.lh", Target: "10.0.0.1", Labels: map[string]string{"pier.upstream": "ftp://:21"}}}, "")

	if w := serve(h, "http://app.lh/", false); w.Code != http.StatusBadGateway {
		t.Errorf("status %d, want 502", w.Code)
	}
}

func TestProxy_Redirect(t *testing.T) {
	t.Parallel()

	records := []Record{{Host: "app.lh", Target: "10.0.0.1", Labels: map[string]string{"pier.redirect": "https"}}}

	w := serve(newTestHandler(records, "443"), "http://app.lh/a?b=1", false)
	if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != "https://app.lh/a?b=1" {
		t.Errorf("443: status %d, location %q", w.Code, w.Header().Get("Location"))
	}

	w = serve(newTestHandler(records, "8443"), "http://app.lh/", false)
	if w.Header().Get("Location") != "https://app.lh:8443/" {
		t.Errorf("8443: location %q", w.Header().Get("Location"))
	}
}

func TestProxy_RedirectIgnoredWithoutHTTPS(t *testing.T) {
	t.Parallel()

	ip, port := backend(t, false)
	h := newTestHandler([]Record{{Host: "app.lh", Target: ip, Labels: map[string]string{"pier.redirect": "https", "pier.upstream": port}}}, "")

	if w := serve(h, "http://app.lh/", false); w.Code != http.StatusOK {
		t.Errorf("status %d, want 200 (redirect ignored when HTTPS is disabled)", w.Code)
	}
}

func TestTargetHost(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"My-App.lh":      "my-app.lh",
		"my-app.lh:8080": "my-app.lh",
		"my-app.lh.":     "my-app.lh",
	}

	for in, want := range cases {
		if got := targetHost(in); got != want {
			t.Errorf("targetHost(%q) = %q, want %q", in, got, want)
		}
	}
}
