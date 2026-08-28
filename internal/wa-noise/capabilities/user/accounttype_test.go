package user

import (
	"context"
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
)

// TestDetectOwnAccountKind_Business: a resposta traz um <verified_name> que
// desserializa — o sinal real e' o certificado presente.
func TestDetectOwnAccountKind_Business(t *testing.T) {
	f := newFakeTransport()
	cert := verifiedNameCertBytes(t, "Acme Inc")
	f.resp = []*waBinary.Node{usyncResponse(usyncUser(userTestPNJID,
		waBinary.Node{Tag: businessNodeTag, Content: []waBinary.Node{
			{Tag: verifiedNameNodeTag, Content: cert},
		}},
	))}

	kind, err := DetectOwnAccountKind(context.Background(), f, userTestPNJID)
	if err != nil {
		t.Fatalf("DetectOwnAccountKind = %v", err)
	}
	if kind != AccountKindBusiness {
		t.Errorf("kind = %v, want AccountKindBusiness", kind)
	}
}

// TestDetectOwnAccountKind_Personal: a resposta nao traz <business>, que e' a
// forma medida (nao suposta) de uma conta pessoal nesta biblioteca — ver o
// comentario de ParseVerifiedName em business.go.
func TestDetectOwnAccountKind_Personal(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(usyncUser(userTestPNJID))}

	kind, err := DetectOwnAccountKind(context.Background(), f, userTestPNJID)
	if err != nil {
		t.Fatalf("DetectOwnAccountKind = %v", err)
	}
	if kind != AccountKindPersonal {
		t.Errorf("kind = %v, want AccountKindPersonal", kind)
	}
}

// TestDetectOwnAccountKind_TransportError: nunca uma classificacao inventada
// quando a rede falha — so' AccountKindUnknown e o erro propagado.
func TestDetectOwnAccountKind_TransportError(t *testing.T) {
	f := newFakeTransport()
	wantErr := errors.New("boom")
	f.err = []error{wantErr}

	kind, err := DetectOwnAccountKind(context.Background(), f, userTestPNJID)
	if err == nil {
		t.Fatal("DetectOwnAccountKind did not propagate transport error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want wrapping %v", err, wantErr)
	}
	if kind != AccountKindUnknown {
		t.Errorf("kind = %v, want AccountKindUnknown on error", kind)
	}
}

// TestDetectOwnAccountKind_MissingSelfInResponse: o servidor responde mas nao
// fala do jid pedido — Unknown, nao panic e nao Personal por omissao.
func TestDetectOwnAccountKind_MissingSelfInResponse(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}

	kind, err := DetectOwnAccountKind(context.Background(), f, userTestPNJID)
	if err == nil {
		t.Fatal("expected error when response omits the requested jid")
	}
	if kind != AccountKindUnknown {
		t.Errorf("kind = %v, want AccountKindUnknown", kind)
	}
}

// TestDetectOwnAccountKind_UnparsableCert: o no <verified_name> existe mas o
// certificado nao desserializa — Unknown, nunca Personal por omissao de dado
// corrompido.
func TestDetectOwnAccountKind_UnparsableCert(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(usyncUser(userTestPNJID,
		waBinary.Node{Tag: businessNodeTag, Content: []waBinary.Node{
			{Tag: verifiedNameNodeTag, Content: []byte("not-a-protobuf-cert")},
		}},
	))}

	kind, err := DetectOwnAccountKind(context.Background(), f, userTestPNJID)
	if err == nil {
		t.Fatal("expected error for unparsable certificate")
	}
	if kind != AccountKindUnknown {
		t.Errorf("kind = %v, want AccountKindUnknown", kind)
	}
}
