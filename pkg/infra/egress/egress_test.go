package egress

import (
	"context"
	"net"
	"testing"
)

func TestIsHTTPURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid http", "http://example.com/webhook", true},
		{"valid https", "https://example.com/webhook", true},
		{"valid with port", "https://example.com:8443/webhook", true},
		{"empty string", "", false},
		{"no scheme", "example.com/webhook", false},
		{"ftp scheme", "ftp://example.com/file", false},
		{"no host", "http://", false},
		{"malformed", "http://[::1", false},
		{"javascript scheme", "javascript:alert(1)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHTTPURL(tt.input); got != tt.want {
				t.Errorf("IsHTTPURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsReservedOrLoopback(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"this network", "0.0.0.0", true},
		{"private 10/8", "10.0.0.1", true},
		{"private 172.16/12", "172.16.0.1", true},
		{"private 192.168/16", "192.168.1.1", true},
		{"CGNAT 100.64/10", "100.64.0.1", true},
		{"link-local v4", "169.254.1.1", true},
		{"link-local v6", "fe80::1", true},
		{"IETF protocol assignments", "192.0.0.1", true},
		{"benchmark testing", "198.18.0.1", true},
		{"unique local v6 (fc00::/7)", "fc00::1", true},
		{"unique local v6 (fd00::/7)", "fd12:3456::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		{"multicast v4", "224.0.0.1", true},
		{"multicast v6", "ff02::1", true},
		{"public v4", "8.8.8.8", false},
		{"public v6", "2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tt.ip)
			}
			if got := IsReservedOrLoopback(ip); got != tt.want {
				t.Errorf("IsReservedOrLoopback(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateOutboundURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"not a URL at all", "not a url", true},
		{"ftp scheme rejected", "ftp://example.com", true},
		{"literal loopback IP", "http://127.0.0.1/webhook", true},
		{"literal private IP 10/8", "http://10.0.0.5:8080/webhook", true},
		{"literal private IP 192.168/16", "http://192.168.1.1/webhook", true},
		{"literal link-local", "http://169.254.169.254/latest/meta-data", true},
		{"literal this-network gap", "http://0.0.0.0:8080/", true},
		{"literal unique-local v6", "http://[fc00::1]/webhook", true},
		{"literal public IP", "http://93.184.216.34/webhook", false},
		{"unresolvable host", "http://this-host-does-not-exist.invalid/webhook", true},
		{"hostname resolving to loopback", "http://localhost:8080/webhook", true},
		{"hostname resolving to a public IP", "https://example.com/webhook", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutboundURL(context.Background(), tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOutboundURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOutboundURL_NoHost(t *testing.T) {
	err := ValidateOutboundURL(context.Background(), "http:///path-no-host")
	if err == nil {
		t.Fatal("expected error for URL with no host, got nil")
	}
}
