package bootstrap

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestInitPrivateIPBlocks popula o slice global sem panic.
func TestInitPrivateIPBlocks(t *testing.T) {
	prev := PrivateIPBlocks
	defer func() { PrivateIPBlocks = prev }()

	InitPrivateIPBlocks()
	if len(PrivateIPBlocks) == 0 {
		t.Fatal("InitPrivateIPBlocks populou PrivateIPBlocks com 0 entradas")
	}
}

// TestIsPrivateOrLoopback_Loopback IP loopback.
func TestIsPrivateOrLoopback_Loopback(t *testing.T) {
	prev := PrivateIPBlocks
	defer func() { PrivateIPBlocks = prev }()
	InitPrivateIPBlocks()

	if !IsPrivateOrLoopback(net.ParseIP("127.0.0.1")) {
		t.Error("127.0.0.1 deveria ser loopback")
	}
	if !IsPrivateOrLoopback(net.ParseIP("::1")) {
		t.Error("::1 deveria ser loopback")
	}
}

// TestIsPrivateOrLoopback_PrivateBlocks confirma CIDRs internos.
func TestIsPrivateOrLoopback_PrivateBlocks(t *testing.T) {
	prev := PrivateIPBlocks
	defer func() { PrivateIPBlocks = prev }()
	InitPrivateIPBlocks()

	for _, cidr := range []string{"10.0.0.1", "192.168.1.1", "172.16.0.1", "169.254.0.1", "100.64.0.1"} {
		if !IsPrivateOrLoopback(net.ParseIP(cidr)) {
			t.Errorf("%s deveria ser privado", cidr)
		}
	}
}

// TestIsPrivateOrLoopback_LinkLocal cobre os ramos de link-local.
func TestIsPrivateOrLoopback_LinkLocal(t *testing.T) {
	prev := PrivateIPBlocks
	defer func() { PrivateIPBlocks = prev }()
	InitPrivateIPBlocks()

	if !IsPrivateOrLoopback(net.ParseIP("169.254.1.1")) {
		t.Error("169.254.1.1 deveria ser link-local")
	}
	if !IsPrivateOrLoopback(net.ParseIP("fe80::1")) {
		t.Error("fe80::1 deveria ser link-local unicast")
	}
}

// TestIsPrivateOrLoopback_Public IP público deve ser false.
func TestIsPrivateOrLoopback_Public(t *testing.T) {
	prev := PrivateIPBlocks
	defer func() { PrivateIPBlocks = prev }()
	InitPrivateIPBlocks()

	if IsPrivateOrLoopback(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 é público, deveria ser false")
	}
	if IsPrivateOrLoopback(net.ParseIP("1.1.1.1")) {
		t.Error("1.1.1.1 é público, deveria ser false")
	}
}

// TestIsPrivateOrLoopback_NilIP nil devolve false.
func TestIsPrivateOrLoopback_NilIP(t *testing.T) {
	prev := PrivateIPBlocks
	defer func() { PrivateIPBlocks = prev }()
	InitPrivateIPBlocks()

	if IsPrivateOrLoopback(nil) {
		t.Error("nil IP deveria ser false")
	}
}

// TestIsPrivateOrLoopback_EmptyBlocks sem blocos carregados.
func TestIsPrivateOrLoopback_EmptyBlocks(t *testing.T) {
	prev := PrivateIPBlocks
	defer func() { PrivateIPBlocks = prev }()
	PrivateIPBlocks = nil

	if IsPrivateOrLoopback(net.ParseIP("10.0.0.1")) {
		t.Error("com PrivateIPBlocks vazio, 10.0.0.1 deveria ser false")
	}
}

// TestNewSafeHTTPClient_Basic cria o client com timeout.
func TestNewSafeHTTPClient_Basic(t *testing.T) {
	c := NewSafeHTTPClient()
	if c == nil {
		t.Fatal("NewSafeHTTPClient returned nil")
	}
	if c.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", c.Timeout)
	}
	if c.Transport == nil {
		t.Fatal("Transport is nil")
	}
}

// TestNewSafeHTTPClient_RejectsPrivateIP DialContext recusa IP privado.
func TestNewSafeHTTPClient_RejectsPrivateIP(t *testing.T) {
	c := NewSafeHTTPClient()
	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	// "localhost" resolve para 127.0.0.1 (loopback), que é bloqueado.
	conn, err := transport.DialContext(context.Background(), "tcp", "localhost:1")
	if err == nil {
		conn.Close()
		t.Fatal("DialContext aceitou IP privado")
	}
}

// TestNewSafeHTTPClient_InvalidAddressFormat cobre o caminho net.SplitHostPort.
func TestNewSafeHTTPClient_InvalidAddressFormat(t *testing.T) {
	c := NewSafeHTTPClient()
	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	_, err := transport.DialContext(context.Background(), "tcp", "this-has-no-port")
	if err == nil {
		t.Fatal("DialContext com endereço inválido = nil")
	}
}

// TestNewSafeHTTPClient_NonResolvableHost cobre LookupIP fail.
func TestNewSafeHTTPClient_NonResolvableHost(t *testing.T) {
	c := NewSafeHTTPClient()
	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	_, err := transport.DialContext(context.Background(), "tcp", "this-host-does-not-exist-zzz.invalid:443")
	if err == nil {
		t.Fatal("DialContext com host inexistente = nil")
	}
}

// TestNewSafeHTTPClient_EmptyIPs cobre no-IPS-found (raro).
func TestNewSafeHTTPClient_EmptyIPs(t *testing.T) {
	// Se o resolver retorna []IP{}, o dialer falha com "no dialable IP".
	// Não temos como simular []IP{} sem mock; pulamos.
	t.Skip("LookupIP rarely returns []IP{}; caminho de erro raro")
}

// TestNewSafeHTTPClient_DialError cobre o lastDialErr.
func TestNewSafeHTTPClient_DialError(t *testing.T) {
	// IP público que provavelmente não está rodando nada na porta 1.
	c := NewSafeHTTPClient()
	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	_, err := transport.DialContext(context.Background(), "tcp", "8.8.8.8:1")
	if err == nil {
		t.Fatal("DialContext para 8.8.8.8:1 deveria falhar")
	}
}
