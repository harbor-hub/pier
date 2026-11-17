package tcp

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// backend starts a TCP server on addr ("127.0.0.1:0" for any port). Each
// connection reads until EOF, then replies "<name>:<data>" and closes, so a
// reply proves that half-close reached the backend.
func backend(t *testing.T, addr, name string) string {
	t.Helper()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", addr, err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				data, _ := io.ReadAll(conn)
				fmt.Fprintf(conn, "%s:%s", name, data)
			}()
		}
	}()

	return ln.Addr().String()
}

// logs collects log lines from a Proxy.
type logs struct {
	mu    sync.Mutex
	lines []string
}

func (l *logs) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func startProxy(t *testing.T, p *Proxy) {
	t.Helper()

	if p.DialTimeout == 0 {
		p.DialTimeout = 2 * time.Second
	}
	if p.Logf == nil {
		p.Logf = (&logs{}).logf
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Shutdown(time.Second) })
}

// roundTrip sends data, half-closes, and returns everything received.
func roundTrip(t *testing.T, addr, data string) (string, error) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	io.WriteString(conn, data)
	conn.(*net.TCPConn).CloseWrite()

	reply, err := io.ReadAll(conn)
	return string(reply), err
}

func TestProxy_RoundTripWithHalfClose(t *testing.T) {
	t.Parallel()

	target := backend(t, "127.0.0.1:0", "echo")
	p := &Proxy{Mappings: []Mapping{{Listen: "127.0.0.1:0", Target: target}}}
	startProxy(t, p)

	reply, err := roundTrip(t, p.Addrs()[0].String(), "ping")
	if err != nil || reply != "echo:ping" {
		t.Errorf("got %q, %v; want echo:ping", reply, err)
	}
}

func TestProxy_SeveralMappings(t *testing.T) {
	t.Parallel()

	a := backend(t, "127.0.0.1:0", "a")
	b := backend(t, "127.0.0.1:0", "b")
	p := &Proxy{Mappings: []Mapping{
		{Listen: "127.0.0.1:0", Target: a},
		{Listen: "127.0.0.1:0", Target: b},
	}}
	startProxy(t, p)

	for i, want := range []string{"a:x", "b:x"} {
		if reply, err := roundTrip(t, p.Addrs()[i].String(), "x"); err != nil || reply != want {
			t.Errorf("mapping %d: got %q, %v; want %q", i, reply, err, want)
		}
	}
}

func TestProxy_FailingTargetDoesNotAffectOthers(t *testing.T) {
	t.Parallel()

	// A port that was free a moment ago: connections to it are refused.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := ln.Addr().String()
	ln.Close()

	alive := backend(t, "127.0.0.1:0", "alive")
	l := &logs{}
	p := &Proxy{
		Mappings: []Mapping{
			{Listen: "127.0.0.1:0", Target: dead},
			{Listen: "127.0.0.1:0", Target: alive},
		},
		Logf: l.logf,
	}
	startProxy(t, p)

	if reply, _ := roundTrip(t, p.Addrs()[0].String(), "x"); reply != "" {
		t.Errorf("dead target: got %q, want the connection closed", reply)
	}
	if !strings.Contains(l.String(), "refused") {
		t.Errorf("dial failure not logged: %q", l)
	}

	if reply, err := roundTrip(t, p.Addrs()[1].String(), "x"); err != nil || reply != "alive:x" {
		t.Errorf("other mapping: got %q, %v", reply, err)
	}
}

func TestProxy_VerboseLogsConnections(t *testing.T) {
	t.Parallel()

	target := backend(t, "127.0.0.1:0", "echo")
	l := &logs{}
	p := &Proxy{Mappings: []Mapping{{Listen: "127.0.0.1:0", Target: target}}, Verbose: true, Logf: l.logf}
	startProxy(t, p)

	roundTrip(t, p.Addrs()[0].String(), "x")
	if !strings.Contains(l.String(), "-> "+target) {
		t.Errorf("connection not logged: %q", l)
	}
}

func TestProxy_DuplicateListen(t *testing.T) {
	t.Parallel()

	p := &Proxy{Mappings: []Mapping{
		{Listen: "127.0.0.1:3306", Target: "a:1"},
		{Listen: "127.0.0.1:3306", Target: "b:1"},
	}}
	if err := p.Start(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("got %v, want a duplicate error", err)
	}
}

func TestProxy_BindFailureLeavesNothingListening(t *testing.T) {
	t.Parallel()

	busy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer busy.Close()

	free, _ := net.Listen("tcp", "127.0.0.1:0")
	freeAddr := free.Addr().String()
	free.Close()

	p := &Proxy{Mappings: []Mapping{
		{Listen: freeAddr, Target: "a:1"},
		{Listen: busy.Addr().String(), Target: "b:1"},
	}}
	if err := p.Start(); err == nil {
		t.Fatal("expected a bind error")
	}

	// The first mapping must have been released again.
	ln, err := net.Listen("tcp", freeAddr)
	if err != nil {
		t.Errorf("first listener still bound after the failed start: %v", err)
	} else {
		ln.Close()
	}
}

func TestProxy_ShutdownClosesOpenConnections(t *testing.T) {
	t.Parallel()

	// A backend that never answers, so the connection stays open.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()

	p := &Proxy{Mappings: []Mapping{{Listen: "127.0.0.1:0", Target: ln.Addr().String()}}, DialTimeout: time.Second, Logf: (&logs{}).logf}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("tcp", p.Addrs()[0].String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(100 * time.Millisecond) // let the proxy connect to the backend

	start := time.Now()
	p.Shutdown(200 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("shutdown took %s", elapsed)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, _ := conn.Read(make([]byte, 1)); n != 0 {
		t.Error("client connection should be closed after shutdown")
	}
	if _, err := net.DialTimeout("tcp", p.Addrs()[0].String(), time.Second); err == nil {
		t.Error("listener should be closed after shutdown")
	}
}
