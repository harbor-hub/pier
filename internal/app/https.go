package app

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	helloTimeout = 10 * time.Second
	dialTimeout  = 5 * time.Second
)

// httpsServer accepts TLS connections and routes each by its SNI: hosts whose
// HTTPS upstream is tls:// are forwarded untouched (passthrough); all others
// are terminated with pier's certificate and served by the HTTP handler.
// Without a certificate only passthrough is possible.
type httpsServer struct {
	store     *Store
	terminate *http.Server // nil without a certificate
	verbose   bool
}

func newHTTPSServer(addr string, store *Store, handler http.Handler, cert *tls.Certificate, verbose bool) *httpsServer {
	s := &httpsServer{store: store, verbose: verbose}

	if cert != nil {
		s.terminate = newHTTPServer(addr, handler)
		s.terminate.TLSConfig = &tls.Config{Certificates: []tls.Certificate{*cert}}
	}

	return s
}

// Serve accepts connections on ln until it fails.
func (s *httpsServer) Serve(ln net.Listener) error {
	var terminated *chanListener

	if s.terminate != nil {
		terminated = newChanListener(ln.Addr())
		defer terminated.Close()

		// ServeTLS adds HTTP/2 and wraps the listener in TLS.
		go s.terminate.ServeTLS(terminated, "", "")
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn, terminated)
	}
}

func (s *httpsServer) handle(conn net.Conn, terminated *chanListener) {
	conn.SetReadDeadline(time.Now().Add(helloTimeout))
	serverName, replay, err := peekClientHello(conn)
	conn.SetReadDeadline(time.Time{})

	if err != nil {
		if s.verbose {
			log.Printf("[HTTPS] %s: not a TLS client hello: %v", conn.RemoteAddr(), err)
		}
		conn.Close()
		return
	}

	host := normalizeHost(serverName)
	known := false

	if host != "" {
		if rec, ok := s.store.Lookup(host); ok {
			known = true
			if up, err := selectUpstream(rec.Labels, "https"); err == nil && up.Scheme == "tls" {
				target := up.Host
				if target == "" {
					target = rec.Target
				}
				s.passthrough(replay, host, net.JoinHostPort(target, up.Port), up.ProxyProtocol)
				return
			}
		}
	}

	if terminated != nil {
		terminated.push(replay)
		return
	}

	switch {
	case host == "":
		log.Printf("[HTTPS] %s: no SNI and no --https-cert to terminate TLS; closing", conn.RemoteAddr())
	case !known:
		log.Printf("[HTTPS] %s: unknown host and no --https-cert to terminate TLS; closing", host)
	default:
		log.Printf("[HTTPS] %s: no tls:// upstream and no --https-cert to terminate TLS; closing", host)
	}
	conn.Close()
}

// passthrough forwards the raw TLS stream between client and target,
// optionally preceded by a PROXY protocol header.
func (s *httpsServer) passthrough(client net.Conn, host, target string, proxyProtocol int) {
	defer client.Close()

	backend, err := net.DialTimeout("tcp", target, dialTimeout)
	if err != nil {
		log.Printf("[HTTPS] %s: %v", host, err)
		return
	}
	defer backend.Close()

	if proxyProtocol != 0 {
		if _, err := backend.Write(proxyProtocolHeader(proxyProtocol, client.RemoteAddr(), client.LocalAddr())); err != nil {
			log.Printf("[HTTPS] %s: PROXY header: %v", host, err)
			return
		}
	}

	if s.verbose {
		log.Printf("[HTTPS] %s -> tls://%s (passthrough%s)", host, target, proxyProtocolString(proxyProtocol))
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(backend, client); closeWrite(backend) }()
	go func() { defer wg.Done(); io.Copy(client, backend); closeWrite(client) }()
	wg.Wait()
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	}
}

// chanListener is a net.Listener fed with already accepted connections.
type chanListener struct {
	addr  net.Addr
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newChanListener(addr net.Addr) *chanListener {
	return &chanListener{addr: addr, conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *chanListener) push(conn net.Conn) {
	select {
	case l.conns <- conn:
	case <-l.done:
		conn.Close()
	}
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *chanListener) Addr() net.Addr { return l.addr }
