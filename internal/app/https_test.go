package app

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// selfSignedCert returns a certificate for the given common name.
func selfSignedCert(t *testing.T, name string) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// startHTTPS runs an httpsServer for records on 127.0.0.1 and returns its address.
func startHTTPS(t *testing.T, records []Record, cert *tls.Certificate) string {
	t.Helper()

	store := newTestStore(&fakeSource{records: records})
	store.Refresh()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	handler := newProxyHandler(store, port, false)
	go newHTTPSServer(ln.Addr().String(), store, handler, cert, false).Serve(ln)

	return ln.Addr().String()
}

// get performs an HTTPS GET of https://<host>/ against addr and returns the
// body and the common name of the certificate the client saw. The client sends
// host as SNI; an IP address as host means no SNI.
func get(addr, host string) (string, string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}

	resp, err := client.Get("https://" + host + "/")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	return string(body), resp.TLS.PeerCertificates[0].Subject.CommonName, nil
}

// tlsBackend starts a TLS server with its own certificate and returns its port.
func tlsBackend(t *testing.T, name, body string) string {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{selfSignedCert(t, name)}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	_, port, _ := net.SplitHostPort(u.Host)

	return port
}

func TestHTTPS_Passthrough(t *testing.T) {
	t.Parallel()

	port := tlsBackend(t, "backend", "from backend")
	pierCert := selfSignedCert(t, "pier")

	addr := startHTTPS(t, []Record{
		{Host: "app.pier", Target: "127.0.0.1", Labels: map[string]string{"pier.upstream.https": "tls://:" + port}},
	}, &pierCert)

	body, cn, err := get(addr, "app.pier")
	if err != nil {
		t.Fatal(err)
	}
	if body != "from backend" || cn != "backend" {
		t.Errorf("got body %q, cert %q; want the backend's own certificate", body, cn)
	}
}

func TestHTTPS_PassthroughWithoutPierCert(t *testing.T) {
	t.Parallel()

	port := tlsBackend(t, "backend", "from backend")
	addr := startHTTPS(t, []Record{
		{Host: "app.pier", Target: "127.0.0.1", Labels: map[string]string{"pier.upstream": "tls://:" + port}},
	}, nil)

	if body, cn, err := get(addr, "app.pier"); err != nil || cn != "backend" || body != "from backend" {
		t.Errorf("got %q, %q, %v", body, cn, err)
	}
}

func TestHTTPS_TerminatesOtherHosts(t *testing.T) {
	t.Parallel()

	ip, port := backend(t, false)
	pierCert := selfSignedCert(t, "pier")

	addr := startHTTPS(t, []Record{
		{Host: "web.pier", Target: ip, Labels: map[string]string{"pier.upstream": port}},
	}, &pierCert)

	body, cn, err := get(addr, "web.pier")
	if err != nil {
		t.Fatal(err)
	}
	if cn != "pier" || !strings.Contains(body, " https ") {
		t.Errorf("got body %q, cert %q; want pier's certificate and X-Forwarded-Proto https", body, cn)
	}
}

func TestHTTPS_NoSNIWithCertIsTerminated(t *testing.T) {
	t.Parallel()

	pierCert := selfSignedCert(t, "pier")
	addr := startHTTPS(t, nil, &pierCert)

	// No SNI: routed by the Host header like before; unknown host -> 404 page.
	body, cn, err := get(addr, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if cn != "pier" || !strings.Contains(body, "unknown host") {
		t.Errorf("got body %q, cert %q", body, cn)
	}
}

func TestHTTPS_NoCertClosesNonPassthrough(t *testing.T) {
	t.Parallel()

	addr := startHTTPS(t, []Record{{Host: "web.pier", Target: "127.0.0.1"}}, nil)

	if _, _, err := get(addr, "web.pier"); err == nil {
		t.Error("expected the connection to be closed: no tls:// upstream and no certificate")
	}
}

func TestHTTPS_NotTLS(t *testing.T) {
	t.Parallel()

	pierCert := selfSignedCert(t, "pier")
	addr := startHTTPS(t, nil, &pierCert)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, _ := conn.Read(make([]byte, 1)); n != 0 {
		t.Error("plain HTTP on the HTTPS port should be closed")
	}
}

func TestHTTPS_ProxyProtocol(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"1", "2"} {
		// The backend records what it receives before the TLS bytes.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })

		got := make(chan []byte, 1)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			r := bufio.NewReader(conn)
			head, _ := r.Peek(64) // the header, then the start of the ClientHello
			got <- append([]byte{}, head...)
		}()

		_, port, _ := net.SplitHostPort(ln.Addr().String())
		addr := startHTTPS(t, []Record{
			{Host: "db.pier", Target: "127.0.0.1", Labels: map[string]string{"pier.upstream.https": "tls://:" + port + "?proxyprotocol=" + version}},
		}, nil)

		go get(addr, "db.pier") // fails once the backend closes; only the header matters

		select {
		case head := <-got:
			if version == "1" && !strings.HasPrefix(string(head), "PROXY TCP4 127.0.0.1 127.0.0.1 ") {
				t.Errorf("v1: backend got %q", head)
			}
			if version == "2" && string(head[:12]) != string(proxyProtocolV2Signature) {
				t.Errorf("v2: backend got % x", head)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("v%s: backend received nothing", version)
		}
	}
}

func TestProxy_PassthroughHostOverHTTP(t *testing.T) {
	t.Parallel()

	h := newTestHandler([]Record{{Host: "app.pier", Target: "10.0.0.1", Labels: map[string]string{"pier.upstream": "tls://:8443"}}}, "")

	w := serve(h, "http://app.pier/", false)
	if w.Code != http.StatusMisdirectedRequest {
		t.Errorf("status %d, want 421", w.Code)
	}
}
