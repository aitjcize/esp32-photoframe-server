package mdns

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// NewTransport creates an HTTP transport that uses the system resolver for
// mDNS (.local) support. It retries resolution to prefer IPv4, and falls back
// to IPv6 link-local with auto-detected zone IDs when no IPv4 is available.
func NewTransport() *http.Transport {
	resolver := &net.Resolver{PreferGo: false} // System resolver for mDNS
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return dialer.DialContext(ctx, network, addr)
			}

			// Retry resolution up to 3 times to get an IPv4 address.
			// mDNS on .local can intermittently return only IPv6 link-local.
			const maxRetries = 3
			var lastIPs []string
			for attempt := range maxRetries {
				ips, err := resolver.LookupHost(ctx, host)
				if err != nil {
					if attempt < maxRetries-1 {
						time.Sleep(500 * time.Millisecond)
						continue
					}
					return nil, err
				}
				lastIPs = ips
				for _, ip := range ips {
					if strings.Contains(ip, ".") {
						return dialer.DialContext(ctx, "tcp4", net.JoinHostPort(ip, port))
					}
				}
				if attempt < maxRetries-1 {
					time.Sleep(500 * time.Millisecond)
				}
			}

			// No IPv4 found. Try IPv6 link-local with zone IDs.
			// Link-local addresses (fe80::) require a zone ID (%interface)
			// to identify which network interface to use.
			for _, ip := range lastIPs {
				parsed := net.ParseIP(ip)
				if parsed == nil {
					continue
				}
				if parsed.IsLinkLocalUnicast() {
					ifaces, _ := net.Interfaces()
					for _, iface := range ifaces {
						if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
							continue
						}
						zonedAddr := net.JoinHostPort(ip+"%"+iface.Name, port)
						conn, err := dialer.DialContext(ctx, "tcp6", zonedAddr)
						if err == nil {
							log.Printf("mDNS: connected to %s via %s%%%s (no IPv4 available)", host, ip, iface.Name)
							return conn, nil
						}
					}
				} else {
					// Non-link-local IPv6, try directly
					conn, err := dialer.DialContext(ctx, "tcp6", net.JoinHostPort(ip, port))
					if err == nil {
						return conn, nil
					}
				}
			}

			return nil, fmt.Errorf("failed to connect to %s: no IPv4 after %d attempts, IPv6 link-local also failed", host, maxRetries)
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}
