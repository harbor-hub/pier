package app

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
)

var errHelloRead = errors.New("client hello read")

// peekClientHello reads the TLS ClientHello from conn and returns the server
// name (SNI, empty if the client sent none) and a conn that replays the bytes
// already read, so the handshake can still be completed or forwarded.
func peekClientHello(conn net.Conn) (string, net.Conn, error) {
	var buf bytes.Buffer
	var serverName string
	var gotHello bool

	// A TLS server over a read-only conn: it parses the ClientHello, then the
	// callback aborts the handshake before anything is written.
	err := tls.Server(readOnlyConn{Conn: conn, r: io.TeeReader(conn, &buf)}, &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			serverName, gotHello = hello.ServerName, true
			return nil, errHelloRead
		},
	}).Handshake()

	if !gotHello {
		return "", nil, err
	}

	return serverName, &replayConn{Conn: conn, r: io.MultiReader(&buf, conn)}, nil
}

// readOnlyConn feeds a TLS server from r and refuses writes.
type readOnlyConn struct {
	net.Conn
	r io.Reader
}

func (c readOnlyConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c readOnlyConn) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }

// replayConn reads the peeked bytes first, then from the underlying conn.
type replayConn struct {
	net.Conn
	r io.Reader
}

func (c *replayConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// CloseWrite half-closes the underlying conn when it supports it.
func (c *replayConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}
