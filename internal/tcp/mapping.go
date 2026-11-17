package tcp

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Mapping forwards connections accepted on Listen to Target.
type Mapping struct {
	Listen string // host:port; an empty host means all addresses
	Target string // host:port, resolved on every connection
}

func (m Mapping) String() string {
	return m.Listen + " -> " + m.Target
}

// ParseMapping parses "<listen>=<host>:<port>". <listen> is a port (all
// addresses) or an address with a port, e.g. "127.0.0.1:3306".
func ParseMapping(s string) (Mapping, error) {
	listen, target, ok := strings.Cut(s, "=")
	if !ok {
		return Mapping{}, fmt.Errorf("invalid mapping %q: want <listen>=<host>:<port>", s)
	}

	if isDigits(listen) {
		listen = ":" + listen
	}
	if _, port, err := net.SplitHostPort(listen); err != nil || !validPort(port) {
		return Mapping{}, fmt.Errorf("invalid mapping %q: listen must be a port or <addr>:<port>", s)
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || !validPort(port) {
		return Mapping{}, fmt.Errorf("invalid mapping %q: target must be <host>:<port>", s)
	}

	return Mapping{Listen: listen, Target: target}, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func validPort(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && n >= 1 && n <= 65535
}
