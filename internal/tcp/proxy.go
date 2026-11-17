// Package tcp forwards TCP connections from local listeners to targets that
// are resolved on every connection.
package tcp

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Proxy runs one listener per mapping; the mappings are independent.
type Proxy struct {
	Mappings    []Mapping
	Resolver    *net.Resolver // nil: system resolver
	DialTimeout time.Duration
	Verbose     bool
	Logf        func(format string, args ...any) // nil: log.Printf

	listeners []net.Listener
	wg        sync.WaitGroup

	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

// Start binds every mapping and accepts connections in the background. If one
// mapping cannot be bound, nothing is left listening.
func (p *Proxy) Start() error {
	seen := map[string]bool{}
	for _, m := range p.Mappings {
		// Port 0 picks a free port on every listen, so it never collides.
		if _, port, _ := net.SplitHostPort(m.Listen); port == "0" {
			continue
		}
		if seen[m.Listen] {
			return fmt.Errorf("duplicate listen address %s", m.Listen)
		}
		seen[m.Listen] = true
	}

	p.conns = map[net.Conn]struct{}{}

	for _, m := range p.Mappings {
		ln, err := net.Listen("tcp", m.Listen)
		if err != nil {
			p.closeListeners()
			return fmt.Errorf("listen %s: %w", m.Listen, err)
		}
		p.listeners = append(p.listeners, ln)
	}

	for i, ln := range p.listeners {
		p.wg.Add(1)
		go p.accept(ln, p.Mappings[i])
	}

	return nil
}

// Addrs returns the bound addresses, in mapping order.
func (p *Proxy) Addrs() []net.Addr {
	addrs := make([]net.Addr, len(p.listeners))
	for i, ln := range p.listeners {
		addrs[i] = ln.Addr()
	}
	return addrs
}

// Shutdown stops accepting, waits up to timeout for open connections to
// finish, then closes the remaining ones.
func (p *Proxy) Shutdown(timeout time.Duration) {
	p.closeListeners()

	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()

	select {
	case <-done:
		return
	case <-time.After(timeout):
	}

	p.mu.Lock()
	for c := range p.conns {
		c.Close()
	}
	p.mu.Unlock()

	<-done
}

func (p *Proxy) accept(ln net.Listener, m Mapping) {
	defer p.wg.Done()

	for {
		client, err := ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				p.logf("[TCP] %s: accept: %v", m, err)
			}
			return
		}

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.forward(client, m)
		}()
	}
}

func (p *Proxy) forward(client net.Conn, m Mapping) {
	p.track(client, true)
	defer p.track(client, false)
	defer client.Close()

	dialer := net.Dialer{Timeout: p.DialTimeout, Resolver: p.Resolver}
	backend, err := dialer.Dial("tcp", m.Target)
	if err != nil {
		p.logf("[TCP] %s: %v", m, err)
		return
	}
	p.track(backend, true)
	defer p.track(backend, false)
	defer backend.Close()

	if p.Verbose {
		p.logf("[TCP] %s -> %s (from %s)", m, backend.RemoteAddr(), client.RemoteAddr())
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(backend, client); closeWrite(backend) }()
	go func() { defer wg.Done(); io.Copy(client, backend); closeWrite(client) }()
	wg.Wait()
}

func (p *Proxy) track(c net.Conn, add bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if add {
		p.conns[c] = struct{}{}
	} else {
		delete(p.conns, c)
	}
}

func (p *Proxy) closeListeners() {
	for _, ln := range p.listeners {
		ln.Close()
	}
}

func (p *Proxy) logf(format string, args ...any) {
	if p.Logf != nil {
		p.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// closeWrite half-closes c so the other side sees EOF while its replies can
// still arrive.
func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	}
}
