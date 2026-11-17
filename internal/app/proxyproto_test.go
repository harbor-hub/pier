package app

import (
	"bytes"
	"net"
	"testing"
)

func tcpAddr(ip string, port int) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}

func TestProxyProtocolV1(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		src, dst net.Addr
		want     string
	}{
		{"IPv4", tcpAddr("192.168.1.10", 51000), tcpAddr("10.0.0.1", 443), "PROXY TCP4 192.168.1.10 10.0.0.1 51000 443\r\n"},
		{"IPv6", tcpAddr("2001:db8::1", 51000), tcpAddr("::1", 443), "PROXY TCP6 2001:db8::1 ::1 51000 443\r\n"},
		{"unknown", &net.UnixAddr{Name: "x"}, &net.UnixAddr{Name: "y"}, "PROXY UNKNOWN\r\n"},
	}

	for _, c := range cases {
		if got := string(proxyProtocolHeader(1, c.src, c.dst)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestProxyProtocolV2_IPv4(t *testing.T) {
	t.Parallel()

	got := proxyProtocolHeader(2, tcpAddr("192.168.1.10", 51000), tcpAddr("10.0.0.1", 443))

	want := append([]byte{}, proxyProtocolV2Signature...)
	want = append(want,
		0x21,       // version 2, PROXY
		0x11,       // TCP over IPv4
		0x00, 0x0c, // 12 bytes of addresses
		192, 168, 1, 10, // src
		10, 0, 0, 1, // dst
		0xc7, 0x38, // 51000
		0x01, 0xbb, // 443
	)

	if !bytes.Equal(got, want) {
		t.Errorf("got  % x\nwant % x", got, want)
	}
}

func TestProxyProtocolV2_IPv6(t *testing.T) {
	t.Parallel()

	got := proxyProtocolHeader(2, tcpAddr("2001:db8::1", 51000), tcpAddr("::1", 443))

	if !bytes.HasPrefix(got, proxyProtocolV2Signature) {
		t.Fatal("missing v2 signature")
	}
	h := got[len(proxyProtocolV2Signature):]
	if h[0] != 0x21 || h[1] != 0x21 || h[2] != 0x00 || h[3] != 36 || len(h) != 4+36 {
		t.Errorf("header % x: want version/command 21, family 21, length 36", h[:4])
	}
}

func TestProxyProtocolV2_Unknown(t *testing.T) {
	t.Parallel()

	got := proxyProtocolHeader(2, &net.UnixAddr{Name: "x"}, &net.UnixAddr{Name: "y"})
	want := append(append([]byte{}, proxyProtocolV2Signature...), 0x20, 0x00, 0x00, 0x00)

	if !bytes.Equal(got, want) {
		t.Errorf("got % x, want LOCAL header % x", got, want)
	}
}
