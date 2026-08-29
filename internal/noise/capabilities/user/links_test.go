package user

import (
	"errors"
	"testing"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// qrResponse monta a resposta de um <iq> de w:qr.
func qrResponse(attrs waBinary.Attrs, children ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{
		Tag:     "iq",
		Content: []waBinary.Node{{Tag: qrNodeTag, Attrs: attrs, Content: children}},
	}
}

// --- ResolveBusinessMessageLink ---

func TestResolveBusinessMessageLinkReadsEveryField(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(
		waBinary.Attrs{"jid": userTestPNJID, "notify": "Loja"},
		waBinary.Node{Tag: "message", Content: []byte("ola, quero comprar")},
		waBinary.Node{Tag: businessNodeTag, Attrs: waBinary.Attrs{
			"is_signed": "true", "verified_name": "Loja LTDA", "verified_level": "high",
		}},
	)}

	got, err := ResolveBusinessMessageLink(t.Context(), f, "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JID != userTestPNJID || got.PushName != "Loja" || got.Message != "ola, quero comprar" {
		t.Errorf("got %+v", got)
	}
	if !got.IsSigned || got.VerifiedName != "Loja LTDA" || got.VerifiedLevel != "high" {
		t.Errorf("got %+v", got)
	}
	iq := f.sent[0]
	if iq.Namespace != qrIQNamespace || iq.Type != IQGet {
		t.Errorf("envelope = %+v", iq)
	}
	// O <iq> de w:qr sai SEM "to" — e' assim que o WhatsApp Android manda.
	if iq.To != types.EmptyJID {
		t.Errorf("to = %s, queria vazio", iq.To)
	}
	if iq.Content.([]waBinary.Node)[0].Attrs["code"] != "abc123" {
		t.Errorf("code = %v", iq.Content)
	}
}

// Os dois prefixos de link sao cortados antes de mandar o codigo.
func TestResolveBusinessMessageLinkTrimsBothPrefixes(t *testing.T) {
	for name, input := range map[string]string{
		"wa.me":        BusinessMessageLinkPrefix + "abc123",
		"api.whatsapp": BusinessMessageLinkDirectPrefix + "abc123",
		"so' o codigo": "abc123",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeTransport()
			f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{"jid": userTestPNJID, "notify": "x"})}
			if _, err := ResolveBusinessMessageLink(t.Context(), f, input); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := f.sent[0].Content.([]waBinary.Node)[0].Attrs["code"]; got != "abc123" {
				t.Errorf("code = %v", got)
			}
		})
	}
}

func TestResolveBusinessMessageLinkMaps404(t *testing.T) {
	f := newFakeTransport()
	f.err = []error{testIQErrors.NotFound}
	_, err := ResolveBusinessMessageLink(t.Context(), f, "x")
	if !errors.Is(err, ErrBusinessMessageLinkNotFound) {
		t.Fatalf("got %v", err)
	}
	if !errors.Is(err, testIQErrors.NotFound) {
		t.Errorf("o erro de IQ original se perdeu: %v", err)
	}
}

func TestResolveBusinessMessageLinkPropagatesOtherErrors(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := ResolveBusinessMessageLink(t.Context(), f, "x"); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestResolveBusinessMessageLinkMissingQRIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := ResolveBusinessMessageLink(t.Context(), f, "x")
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != qrNodeTag {
		t.Errorf("got %v", err)
	}
}

// <message> e <business> sao opcionais: sem eles o alvo sai com os campos
// zerados, sem erro.
func TestResolveBusinessMessageLinkOptionalChildren(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{"jid": userTestPNJID, "notify": "Loja"})}
	got, err := ResolveBusinessMessageLink(t.Context(), f, "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Message != "" || got.IsSigned || got.VerifiedName != "" {
		t.Errorf("got %+v", got)
	}
}

// <message> com filhos em vez de bytes nao vira panic.
func TestResolveBusinessMessageLinkNonByteMessage(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(
		waBinary.Attrs{"jid": userTestPNJID, "notify": "Loja"},
		waBinary.Node{Tag: "message", Content: []waBinary.Node{{Tag: "x"}}},
	)}
	got, err := ResolveBusinessMessageLink(t.Context(), f, "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Message != "" {
		t.Errorf("message = %q", got.Message)
	}
}

// Atributo obrigatorio ausente devolve o alvo PARCIAL junto do erro.
func TestResolveBusinessMessageLinkReturnsPartialOnAttrError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{"jid": userTestPNJID})}
	got, err := ResolveBusinessMessageLink(t.Context(), f, "x")
	if err == nil {
		t.Fatal("esperava erro de atributo ausente (notify)")
	}
	if got == nil || got.JID != userTestPNJID {
		t.Errorf("got %+v", got)
	}
}

// --- ResolveContactQRLink ---

func TestResolveContactQRLinkReadsEveryField(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{
		"jid": userTestPNJID, "notify": "Alice", "type": qrTypeContact,
	})}

	got, err := ResolveContactQRLink(t.Context(), f, ContactQRLinkPrefix+"abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JID != userTestPNJID || got.PushName != "Alice" || got.Type != qrTypeContact {
		t.Errorf("got %+v", got)
	}
	if f.sent[0].Content.([]waBinary.Node)[0].Attrs["code"] != "abc" {
		t.Errorf("code = %v", f.sent[0].Content)
	}
}

func TestResolveContactQRLinkTrimsDirectPrefix(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{"jid": userTestPNJID, "type": qrTypeContact})}
	if _, err := ResolveContactQRLink(t.Context(), f, ContactQRLinkDirectPrefix+"abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.sent[0].Content.([]waBinary.Node)[0].Attrs["code"] != "abc" {
		t.Errorf("code = %v", f.sent[0].Content)
	}
}

// notify e' opcional aqui (ao contrario do link business): sem ele nao ha' erro.
func TestResolveContactQRLinkNotifyIsOptional(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{"jid": userTestPNJID, "type": qrTypeContact})}
	got, err := ResolveContactQRLink(t.Context(), f, "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PushName != "" {
		t.Errorf("push name = %q", got.PushName)
	}
}

func TestResolveContactQRLinkMaps404(t *testing.T) {
	f := newFakeTransport()
	f.err = []error{testIQErrors.NotFound}
	_, err := ResolveContactQRLink(t.Context(), f, "x")
	if !errors.Is(err, ErrContactQRLinkNotFound) || !errors.Is(err, testIQErrors.NotFound) {
		t.Errorf("got %v", err)
	}
}

func TestResolveContactQRLinkPropagatesOtherErrors(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := ResolveContactQRLink(t.Context(), f, "x"); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestResolveContactQRLinkMissingQRIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := ResolveContactQRLink(t.Context(), f, "x")
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.In != "response to contact link query" {
		t.Errorf("got %v", err)
	}
}

// --- GetContactQRLink ---

func TestGetContactQRLinkGetAndRevoke(t *testing.T) {
	for name, tc := range map[string]struct {
		revoke bool
		want   string
	}{
		"get":    {false, qrActionGet},
		"revoke": {true, qrActionRevoke},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeTransport()
			f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{"code": "meu-codigo"})}
			got, err := GetContactQRLink(t.Context(), f, tc.revoke)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != "meu-codigo" {
				t.Errorf("got %q", got)
			}
			iq := f.sent[0]
			if iq.Namespace != qrIQNamespace || iq.Type != IQSet {
				t.Errorf("envelope = %+v", iq)
			}
			attrs := iq.Content.([]waBinary.Node)[0].Attrs
			if attrs["type"] != qrTypeContact || attrs["action"] != tc.want {
				t.Errorf("attrs = %v", attrs)
			}
		})
	}
}

func TestGetContactQRLinkPropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	got, err := GetContactQRLink(t.Context(), f, false)
	if !errors.Is(err, boom) || got != "" {
		t.Errorf("got (%q, %v)", got, err)
	}
}

func TestGetContactQRLinkMissingQRIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	got, err := GetContactQRLink(t.Context(), f, false)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.In != "response to own contact link fetch" {
		t.Errorf("got %v", err)
	}
	if got != "" {
		t.Errorf("got %q", got)
	}
}

// `code` ausente devolve string vazia junto do erro de atributo.
func TestGetContactQRLinkMissingCodeAttr(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{qrResponse(waBinary.Attrs{})}
	got, err := GetContactQRLink(t.Context(), f, false)
	if err == nil {
		t.Fatal("esperava erro de atributo ausente")
	}
	if got != "" {
		t.Errorf("got %q", got)
	}
}
