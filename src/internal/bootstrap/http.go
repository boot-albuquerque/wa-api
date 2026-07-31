package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// PrivateIPBlocks holds the CIDR blocks considered internal/non-routable.
var PrivateIPBlocks []*net.IPNet

// InitPrivateIPBlocks populates the global PrivateIPBlocks slice.
func InitPrivateIPBlocks() {
	for _, cidr := range []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12",
		"192.168.0.0/16", "100.64.0.0/10", "169.254.0.0/16",
		"::1/128", "fe80::/10",
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to parse CIDR string: %s", cidr)
		}
		PrivateIPBlocks = append(PrivateIPBlocks, block)
	}
}

// IsPrivateOrLoopback returns true if ip is private or loopback.
func IsPrivateOrLoopback(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	for _, block := range PrivateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// NewSafeHTTPClient returns an SSRF-safe *http.Client.
func NewSafeHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, fmt.Errorf("unexpected address format: %q: %w", addr, err)
				}
				ips, err := net.LookupIP(host)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve host '%s': %w", host, err)
				}
				if len(ips) == 0 {
					return nil, fmt.Errorf("no IP addresses found for host: %s", host)
				}
				var lastDialErr error
				for _, ip := range ips {
					if IsPrivateOrLoopback(ip) {
						continue
					}
					d := &net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}
					conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
					if err == nil {
						return conn, nil
					}
					lastDialErr = err
				}
				if lastDialErr != nil {
					return nil, lastDialErr
				}
				return nil, fmt.Errorf("no dialable IP for %s", host)
			},
		},
	}
}
