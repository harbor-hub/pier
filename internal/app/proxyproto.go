package app

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

// proxyProtocolV2Signature starts every PROXY protocol v2 header.
var proxyProtocolV2Signature = []byte("\r\n\r\n\x00\r\nQUIT\n")

// proxyProtocolHeader returns the PROXY protocol header (version 1 or 2) that
// tells the backend the client's address (src) and the address the client
// connected to (dst).
func proxyProtocolHeader(version int, src, dst net.Addr) []byte {
	s, sok := src.(*net.TCPAddr)
	d, dok := dst.(*net.TCPAddr)
	known := sok && dok

	v4 := known && s.IP.To4() != nil && d.IP.To4() != nil

	if version == 1 {
		switch {
		case !known:
			return []byte("PROXY UNKNOWN\r\n")
		case v4:
			return fmt.Appendf(nil, "PROXY TCP4 %s %s %d %d\r\n", s.IP.To4(), d.IP.To4(), s.Port, d.Port)
		default:
			return fmt.Appendf(nil, "PROXY TCP6 %s %s %d %d\r\n", s.IP.To16(), d.IP.To16(), s.Port, d.Port)
		}
	}

	h := append([]byte{}, proxyProtocolV2Signature...)

	if !known {
		// LOCAL command, unspecified family, no addresses.
		return append(h, 0x20, 0x00, 0x00, 0x00)
	}

	var addrs []byte
	family := byte(0x21) // TCP over IPv6
	if v4 {
		family = 0x11 // TCP over IPv4
		addrs = append(append(addrs, s.IP.To4()...), d.IP.To4()...)
	} else {
		addrs = append(append(addrs, s.IP.To16()...), d.IP.To16()...)
	}
	addrs = binary.BigEndian.AppendUint16(addrs, uint16(s.Port))
	addrs = binary.BigEndian.AppendUint16(addrs, uint16(d.Port))

	h = append(h, 0x21, family) // version 2 + PROXY command, family
	h = binary.BigEndian.AppendUint16(h, uint16(len(addrs)))

	return append(h, addrs...)
}

// proxyProtocolString is used in logs.
func proxyProtocolString(version int) string {
	if version == 0 {
		return ""
	}
	return " + PROXY v" + strconv.Itoa(version)
}
