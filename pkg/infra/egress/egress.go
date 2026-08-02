// Package egress is the single validator for outbound URLs the user
// configures (webhook, proxy, S3 endpoint): is it a well-formed http(s)
// URL, and does it resolve to an address this process should be allowed to
// dial? Consolidates the two copies of IsHTTPURL that existed before
// (pkg/infra/helpers/pure.go and pkg/infra/media/media_utils.go, the
// latter removed in Fase 2) and closes the gaps in the private/reserved IP
// table that pkg/bootstrap/http.go's version left open: 0.0.0.0/8,
// fc00::/7, 192.0.0.0/24, 198.18.0.0/15, multicast, broadcast, and
// IsUnspecified() were not checked.
package egress

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/rs/zerolog/log"
)

// IsHTTPURL reports whether s is a syntactically valid http/https URL with
// a non-empty host.
func IsHTTPURL(s string) bool {
	parsed, err := url.ParseRequestURI(s)
	if err != nil {
		// A URL bruta nunca entra no registro: ela pode carregar credencial
		// no userinfo ou na query. Só o motivo da recusa é observável.
		log.Debug().Err(err).Str("reason", "unparseable").
			Msg("URL rejected as non-http(s)")
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		log.Debug().Str("reason", "scheme").Str("scheme", parsed.Scheme).
			Msg("URL rejected as non-http(s)")
		return false
	}
	if parsed.Host == "" {
		log.Debug().Str("reason", "empty_host").
			Msg("URL rejected as non-http(s)")
		return false
	}
	return true
}

// reservedBlocks are the CIDR ranges no outbound URL should resolve to.
// Superset of pkg/bootstrap/http.go's PrivateIPBlocks: adds 0.0.0.0/8
// (this network), fc00::/7 (IPv6 unique local, the v6 analog of RFC1918),
// 192.0.0.0/24 (IETF protocol assignments), and 198.18.0.0/15 (benchmark
// testing) — none of these were blocked before.
var reservedCIDRs = []string{
	"0.0.0.0/8",
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"169.254.0.0/16",
	"192.0.0.0/24",
	"198.18.0.0/15",
	"::1/128",
	"fe80::/10",
	"fc00::/7",
}

var reservedBlocks = mustParseCIDRs(reservedCIDRs)

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	blocks := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			// O panic aborta o processo na inicialização do pacote, quando
			// nada leu ainda o stderr estruturado; o registro existe para
			// que a causa apareça no mesmo formato do resto do sistema.
			log.Error().Err(err).
				Str("cidr", cidr).
				Msg("invalid CIDR literal in the reserved-block table")
			panic(fmt.Sprintf("egress: invalid CIDR literal %q: %v", cidr, err))
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// IsReservedOrLoopback reports whether ip is loopback, link-local,
// multicast, unspecified (0.0.0.0 / ::), or falls in one of reservedCIDRs.
// This is the check pkg/bootstrap/http.go's IsPrivateOrLoopback should
// have been — see the package doc for exactly which ranges it was missing.
func IsReservedOrLoopback(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		log.Debug().Stringer("ip", ip).Str("reason", "special_purpose").
			Msg("address classified as reserved")
		return true
	}
	for _, block := range reservedBlocks {
		if block.Contains(ip) {
			log.Debug().Stringer("ip", ip).Str("reason", "reserved_cidr").
				Str("block", block.String()).
				Msg("address classified as reserved")
			return true
		}
	}
	return false
}

// ValidateOutboundURL checks that rawURL is a well-formed http(s) URL and
// that every IP its host resolves to is a routable, non-reserved address.
// Intended for user-configured destinations (webhook URL, proxy URL, S3
// endpoint) validated once at configuration time — it is not a
// per-request SSRF guard on the dial path itself (see
// pkg/bootstrap/http.go's NewSafeHTTPClient for that layer, which blocks
// at DialContext regardless of what was validated at config time).
func ValidateOutboundURL(ctx context.Context, rawURL string) error {
	if !IsHTTPURL(rawURL) {
		log.Warn().Str("reason", "malformed").Msg("outbound URL rejected")
		return fmt.Errorf("not a valid http(s) URL")
	}

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		log.Warn().Err(err).Str("reason", "unparseable").Msg("outbound URL rejected")
		return fmt.Errorf("not a valid http(s) URL")
	}

	host := parsed.Hostname()
	if host == "" {
		log.Warn().Str("reason", "empty_host").Msg("outbound URL rejected")
		return fmt.Errorf("URL has no host")
	}

	if ip := net.ParseIP(host); ip != nil {
		if IsReservedOrLoopback(ip) {
			log.Warn().Str("reason", "reserved_address").Str("host", host).
				Msg("outbound URL rejected")
			return fmt.Errorf("URL resolves to a reserved or loopback address")
		}
		return nil
	}

	resolver := &net.Resolver{}
	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		log.Warn().Err(err).Str("reason", "unresolvable").Str("host", host).
			Msg("outbound URL rejected")
		return fmt.Errorf("could not resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		log.Warn().Str("reason", "no_addresses").Str("host", host).
			Msg("outbound URL rejected")
		return fmt.Errorf("no IP addresses found for host %q", host)
	}
	for _, ip := range ips {
		if IsReservedOrLoopback(ip) {
			log.Warn().Str("reason", "reserved_address").Str("host", host).
				Stringer("resolved_ip", ip).
				Msg("outbound URL rejected")
			return fmt.Errorf("URL resolves to a reserved or loopback address")
		}
	}
	return nil
}
