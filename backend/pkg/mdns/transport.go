package mdns

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// NewTransport creates an HTTP transport that uses the system resolver for
// mDNS (.local) support and retries resolution until an IPv4 address is found.
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
			for attempt := range maxRetries {
				ips, err := resolver.LookupHost(ctx, host)
				if err != nil {
					if attempt < maxRetries-1 {
						time.Sleep(500 * time.Millisecond)
						continue
					}
					return nil, err
				}
				for _, ip := range ips {
					if strings.Contains(ip, ".") {
						return dialer.DialContext(ctx, "tcp4", net.JoinHostPort(ip, port))
					}
				}
				if attempt < maxRetries-1 {
					time.Sleep(500 * time.Millisecond)
				}
			}
			return nil, fmt.Errorf("no IPv4 address found for %s after %d attempts", host, maxRetries)
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}
