package accounttype

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/domain"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"
)

// TestDetect_NoSession: sem cliente para o txtID, nunca uma classificacao
// inventada.
func TestDetect_NoSession(t *testing.T) {
	d := NewDetector(func(string) client.Client { return nil })

	if err := d.EnsureSession(context.Background(), "user-1"); err == nil {
		t.Fatal("EnsureSession did not report the missing session")
	}

	kind, err := d.Detect(context.Background(), "user-1")
	if err == nil {
		t.Fatal("Detect did not propagate the missing-session error")
	}
	if kind != domain.AccountTypeUnknown {
		t.Errorf("kind = %v, want AccountTypeUnknown", kind)
	}
}

// TestDetect_UnsupportedClientType: um client.Client que nao envolve
// *noise.Client (o duble de teste, aqui) nunca deve virar personal por
// omissao — o adaptador nao tem como medir nada dele.
func TestDetect_UnsupportedClientType(t *testing.T) {
	fake := &testkit.Fake{}
	d := NewDetector(func(string) client.Client { return fake })

	kind, err := d.Detect(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error for a client that is not client.RealClient")
	}
	if !errors.Is(err, errUnsupportedClient) {
		t.Errorf("error = %v, want errUnsupportedClient", err)
	}
	if kind != domain.AccountTypeUnknown {
		t.Errorf("kind = %v, want AccountTypeUnknown", kind)
	}
}
