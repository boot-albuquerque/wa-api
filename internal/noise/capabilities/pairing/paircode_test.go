package pairing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"go.mau.fi/util/random"
	"golang.org/x/crypto/pbkdf2"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/security/keys"
)

// --------------------------------------------------------------- PairPhone

// linkRegResponse monta a resposta de <iq> do estagio companion_hello.
func linkRegResponse(ref any) *waBinary.Node {
	child := waBinary.Node{Tag: "link_code_companion_reg"}
	if ref != nil {
		child.Content = []waBinary.Node{{Tag: "link_code_pairing_ref", Content: ref}}
	}
	return &waBinary.Node{Tag: "iq", Content: []waBinary.Node{child}}
}

func TestPairPhone(t *testing.T) {
	tr := newFakeTransport(t)
	tr.enqueueIQ(linkRegResponse([]byte("ref-do-servidor")), nil)

	code, err := PairPhone(t.Context(), tr, "+55 (11) 99999-9999", true, ClientChrome, "Chrome (Linux)")
	if err != nil {
		t.Fatalf("PairPhone: %v", err)
	}

	// O codigo exibido tem 8 caracteres com hifen no meio.
	if len(code) != 9 || code[codeGroupLength] != '-' {
		t.Errorf("codigo = %q, esperado 8 caracteres com hifen apos %d", code, codeGroupLength)
	}
	for _, c := range strings.ReplaceAll(code, "-", "") {
		if !strings.ContainsRune(codeBase32Alphabet, c) {
			t.Errorf("codigo %q usa caractere %q fora do alfabeto do WhatsApp", code, c)
		}
	}

	cache := tr.State().Linking()
	if cache == nil {
		t.Fatal("a sessao de pareamento pendente nao foi registrada")
	}
	if cache.PairingRef != "ref-do-servidor" {
		t.Errorf("PairingRef = %q", cache.PairingRef)
	}
	if cache.LinkingCode != strings.ReplaceAll(code, "-", "") {
		t.Errorf("LinkingCode = %q, esperado o codigo sem hifen (%q)", cache.LinkingCode, code)
	}
	// O JID e' montado com o telefone limpo de nao-digitos.
	if cache.JID.User != "5511999999999" {
		t.Errorf("JID.User = %q, esperado o telefone so' com digitos", cache.JID.User)
	}

	call := tr.snapshotIQs()[0]
	if call.Namespace != "md" || call.Type != IQSet || call.To != types.ServerJID {
		t.Errorf("IQ = %+v, esperado set/md para o servidor", call)
	}
	reg := call.Content.([]waBinary.Node)[0]
	if reg.Attrs["stage"] != "companion_hello" {
		t.Errorf("stage = %v, esperado companion_hello", reg.Attrs["stage"])
	}
	if reg.Attrs["should_show_push_notification"] != "true" {
		t.Errorf("should_show_push_notification = %v, esperado \"true\"", reg.Attrs["should_show_push_notification"])
	}
	children := reg.Content.([]waBinary.Node)
	if got := children[0].Content.([]byte); len(got) != codeWrappedKeyEnd {
		t.Errorf("chave efemera embrulhada tem %d bytes, esperado %d", len(got), codeWrappedKeyEnd)
	}
	if children[2].Content != string(ClientChrome) {
		t.Errorf("companion_platform_id = %v, esperado %q", children[2].Content, ClientChrome)
	}
}

func TestPairPhoneRejeitaTelefonesInvalidos(t *testing.T) {
	for name, tc := range map[string]struct {
		phone   string
		wantErr error
	}{
		"curto demais":  {"12345", ErrPhoneNumberTooShort},
		"so' simbolos":  {"+-() ", ErrPhoneNumberTooShort},
		"nacional (0…)": {"0119999999", ErrPhoneNumberIsNotInternational},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport(t)

			_, err := PairPhone(t.Context(), tr, tc.phone, false, ClientChrome, "Chrome (Linux)")

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("erro = %v, esperado %v", err, tc.wantErr)
			}
			if len(tr.snapshotIQs()) != 0 {
				t.Error("nao deveria ter tocado no socket")
			}
		})
	}
}

func TestPairPhoneErrosDeResposta(t *testing.T) {
	t.Run("erro de transporte", func(t *testing.T) {
		tr := newFakeTransport(t)
		sentinel := errors.New("socket fechado")
		tr.enqueueIQ(nil, sentinel)

		_, err := PairPhone(t.Context(), tr, "5511999999999", false, ClientChrome, "Chrome (Linux)")
		if !errors.Is(err, sentinel) {
			t.Fatalf("erro = %v, esperado %v", err, sentinel)
		}
	})

	t.Run("sem link_code_pairing_ref", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.enqueueIQ(linkRegResponse(nil), nil)

		_, err := PairPhone(t.Context(), tr, "5511999999999", false, ClientChrome, "Chrome (Linux)")
		var missing *missingElementError
		if !errors.As(err, &missing) || missing.Tag != "link_code_pairing_ref" {
			t.Fatalf("erro = %v (%T), esperado ElementMissing de link_code_pairing_ref", err, err)
		}
	})

	t.Run("ref com tipo inesperado", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.enqueueIQ(linkRegResponse("nao e bytes"), nil)

		_, err := PairPhone(t.Context(), tr, "5511999999999", false, ClientChrome, "Chrome (Linux)")
		if err == nil || !strings.Contains(err.Error(), "unexpected type") {
			t.Fatalf("erro = %v, esperado reclamar do tipo do conteudo", err)
		}
	})
}

// ------------------------------------------------- HandleCodeNotification

// primaryEphemeralBlob simula o que o aparelho principal manda: salt || IV ||
// pubkey efemera cifrada com a chave derivada do codigo de pareamento.
func primaryEphemeralBlob(t *testing.T, linkingCode string) (blob []byte, priv *keys.KeyPair) {
	t.Helper()
	priv = keys.NewKeyPair()
	salt := random.Bytes(codeSaltLength)
	iv := random.Bytes(codeIVLength)
	key := pbkdf2.Key([]byte(linkingCode), salt, codePBKDF2Iterations, codeKeyLength, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	encrypted := make([]byte, codeKeyLength)
	cipher.NewCTR(block, iv).XORKeyStream(encrypted, priv.Pub[:])

	blob = make([]byte, 0, codeWrappedKeyEnd)
	blob = append(blob, salt...)
	blob = append(blob, iv...)
	blob = append(blob, encrypted...)
	return blob, priv
}

// lowOrderEphemeralBlob e' como primaryEphemeralBlob, mas embrulha um ponto de
// ordem baixa (todos-zeros) em vez de uma pubkey valida.
func lowOrderEphemeralBlob(t *testing.T, linkingCode string) []byte {
	t.Helper()
	salt := random.Bytes(codeSaltLength)
	iv := random.Bytes(codeIVLength)
	key := pbkdf2.Key([]byte(linkingCode), salt, codePBKDF2Iterations, codeKeyLength, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	encrypted := make([]byte, codeKeyLength)
	cipher.NewCTR(block, iv).XORKeyStream(encrypted, make([]byte, codeKeyLength))

	blob := make([]byte, 0, codeWrappedKeyEnd)
	blob = append(blob, salt...)
	blob = append(blob, iv...)
	return append(blob, encrypted...)
}

type notificationOpts struct {
	ref            []byte
	wrappedPub     any
	primaryIdenity any
	omitRegNode    bool
}

func notificationNode(opts notificationOpts) *waBinary.Node {
	if opts.omitRegNode {
		return &waBinary.Node{Tag: "notification"}
	}
	children := []waBinary.Node{
		{Tag: "link_code_pairing_ref", Content: opts.ref},
	}
	if opts.wrappedPub != nil {
		children = append(children, waBinary.Node{
			Tag: "link_code_pairing_wrapped_primary_ephemeral_pub", Content: opts.wrappedPub,
		})
	}
	if opts.primaryIdenity != nil {
		children = append(children, waBinary.Node{Tag: "primary_identity_pub", Content: opts.primaryIdenity})
	}
	return &waBinary.Node{Tag: "notification", Content: []waBinary.Node{
		{Tag: "link_code_companion_reg", Content: children},
	}}
}

// pendingPairing deixa o transporte com uma sessao de pareamento por codigo
// aberta, como PairPhone deixaria.
func pendingPairing(t *testing.T, tr *fakeTransport) *LinkingCache {
	t.Helper()
	cache := &LinkingCache{
		JID:         types.NewJID("5511999999999", types.DefaultUserServer),
		KeyPair:     keys.NewKeyPair(),
		LinkingCode: "ABCD2345",
		PairingRef:  "ref-do-servidor",
	}
	tr.State().SetLinking(cache)
	return cache
}

func TestHandleCodeNotification(t *testing.T) {
	tr := newFakeTransport(t)
	cache := pendingPairing(t, tr)
	blob, _ := primaryEphemeralBlob(t, cache.LinkingCode)
	primaryIdentity := keys.NewKeyPair()
	tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)

	err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{
		ref:            []byte(cache.PairingRef),
		wrappedPub:     blob,
		primaryIdenity: primaryIdentity.Pub[:],
	}))
	if err != nil {
		t.Fatalf("HandleCodeNotification: %v", err)
	}

	if len(tr.Store().AdvSecretKey) != codeAdvSecretLength {
		t.Errorf("AdvSecretKey tem %d bytes, esperado %d", len(tr.Store().AdvSecretKey), codeAdvSecretLength)
	}

	call := tr.snapshotIQs()[0]
	reg := call.Content.([]waBinary.Node)[0]
	if reg.Attrs["stage"] != "companion_finish" {
		t.Errorf("stage = %v, esperado companion_finish", reg.Attrs["stage"])
	}
	if reg.Attrs["jid"] != cache.JID {
		t.Errorf("jid = %v, esperado o da sessao pendente (%v)", reg.Attrs["jid"], cache.JID)
	}
	children := reg.Content.([]waBinary.Node)
	bundle := children[0].Content.([]byte)
	// salt || nonce || (identidade companion + identidade primaria + random) + tag GCM
	wantLen := codeKeyBundleSaltLength + codeKeyBundleNonceLength +
		codeKeyLength + codeKeyLength + codeAdvSecretRandomLength + 16
	if len(bundle) != wantLen {
		t.Errorf("key bundle tem %d bytes, esperado %d", len(bundle), wantLen)
	}
	if string(children[2].Content.([]byte)) != cache.PairingRef {
		t.Errorf("link_code_pairing_ref = %q, deveria ecoar o da notificacao", children[2].Content)
	}
}

func TestHandleCodeNotificationErros(t *testing.T) {
	t.Run("sem link_code_companion_reg", func(t *testing.T) {
		tr := newFakeTransport(t)
		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{omitRegNode: true}))
		var missing *missingElementError
		if !errors.As(err, &missing) || missing.Tag != "link_code_companion_reg" {
			t.Fatalf("erro = %v (%T)", err, err)
		}
	})

	t.Run("sem pareamento pendente", func(t *testing.T) {
		tr := newFakeTransport(t)
		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{ref: []byte("x")}))
		if !errors.Is(err, ErrNoPendingPairing) {
			t.Fatalf("erro = %v, esperado ErrNoPendingPairing", err)
		}
	})

	t.Run("ref de outra sessao", func(t *testing.T) {
		tr := newFakeTransport(t)
		pendingPairing(t, tr)
		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{ref: []byte("outra-ref")}))
		if !errors.Is(err, ErrPairingRefMismatch) {
			t.Fatalf("erro = %v, esperado ErrPairingRefMismatch", err)
		}
	})

	t.Run("sem chave efemera do primario", func(t *testing.T) {
		tr := newFakeTransport(t)
		cache := pendingPairing(t, tr)
		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{ref: []byte(cache.PairingRef)}))
		var missing *missingElementError
		if !errors.As(err, &missing) || missing.Tag != "link_code_pairing_wrapped_primary_ephemeral_pub" {
			t.Fatalf("erro = %v (%T)", err, err)
		}
	})

	t.Run("chave efemera curta", func(t *testing.T) {
		tr := newFakeTransport(t)
		cache := pendingPairing(t, tr)
		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{
			ref: []byte(cache.PairingRef), wrappedPub: []byte{1, 2, 3},
		}))
		if err == nil || !strings.Contains(err.Error(), "unexpected length") {
			t.Fatalf("erro = %v, esperado reclamar do comprimento", err)
		}
	})

	t.Run("sem primary_identity_pub", func(t *testing.T) {
		tr := newFakeTransport(t)
		cache := pendingPairing(t, tr)
		blob, _ := primaryEphemeralBlob(t, cache.LinkingCode)
		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{
			ref: []byte(cache.PairingRef), wrappedPub: blob,
		}))
		var missing *missingElementError
		if !errors.As(err, &missing) || missing.Tag != "primary_identity_pub" {
			t.Fatalf("erro = %v (%T)", err, err)
		}
	})

	// X25519 recusa uma chave publica de comprimento errado; e' o unico erro
	// alcancavel do calculo de segredo compartilhado.
	t.Run("identidade primaria de tamanho invalido", func(t *testing.T) {
		tr := newFakeTransport(t)
		cache := pendingPairing(t, tr)
		blob, _ := primaryEphemeralBlob(t, cache.LinkingCode)
		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{
			ref: []byte(cache.PairingRef), wrappedPub: blob, primaryIdenity: []byte{1, 2, 3},
		}))
		if err == nil || !strings.Contains(err.Error(), "identity shared key") {
			t.Fatalf("erro = %v, esperado falha no segredo compartilhado de identidade", err)
		}
	})

	// O X25519 recusa pontos de ordem baixa; o de todos-zeros e' um deles.
	// Cifrando 32 bytes zerados no lugar da pubkey efemera, o segredo
	// compartilhado efemero falha antes do de identidade.
	t.Run("chave efemera de ordem baixa", func(t *testing.T) {
		tr := newFakeTransport(t)
		cache := pendingPairing(t, tr)
		blob := lowOrderEphemeralBlob(t, cache.LinkingCode)

		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{
			ref: []byte(cache.PairingRef), wrappedPub: blob, primaryIdenity: keys.NewKeyPair().Pub[:],
		}))
		if err == nil || !strings.Contains(err.Error(), "ephemeral shared secret") {
			t.Fatalf("erro = %v, esperado falha no segredo compartilhado efemero", err)
		}
	})

	t.Run("erro no IQ final", func(t *testing.T) {
		tr := newFakeTransport(t)
		cache := pendingPairing(t, tr)
		blob, _ := primaryEphemeralBlob(t, cache.LinkingCode)
		sentinel := errors.New("socket fechado")
		tr.enqueueIQ(nil, sentinel)

		err := HandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{
			ref: []byte(cache.PairingRef), wrappedPub: blob, primaryIdenity: keys.NewKeyPair().Pub[:],
		}))
		if !errors.Is(err, sentinel) {
			t.Fatalf("erro = %v, esperado %v", err, sentinel)
		}
	})
}

// TryHandleCodeNotification apenas loga: o contrato e' nao propagar nem entrar
// em panico.
func TestTryHandleCodeNotificationEngoleOErro(t *testing.T) {
	tr := newFakeTransport(t)
	TryHandleCodeNotification(t.Context(), tr, notificationNode(notificationOpts{omitRegNode: true}))
}

// ---------------------------------------------------------- estado e chaves

func TestStateLinkingComecaVazio(t *testing.T) {
	var s State
	if s.Linking() != nil {
		t.Error("o zero value de State deveria ter sessao pendente nil")
	}
	cache := &LinkingCache{PairingRef: "x"}
	s.SetLinking(cache)
	if s.Linking() != cache {
		t.Error("SetLinking/Linking deveriam guardar o mesmo ponteiro")
	}
}

// A chave efemera embrulhada tem exatamente salt || IV || pubkey cifrada, e a
// pubkey cifrada NAO pode ser igual a' clara.
func TestGenerateCompanionEphemeralKey(t *testing.T) {
	kp, ephemeralKey, code := generateCompanionEphemeralKey()

	if len(ephemeralKey) != codeWrappedKeyEnd {
		t.Fatalf("blob tem %d bytes, esperado %d", len(ephemeralKey), codeWrappedKeyEnd)
	}
	if len(code) != 2*codeGroupLength {
		t.Errorf("codigo = %q, esperado %d caracteres", code, 2*codeGroupLength)
	}
	// generateCompanionEphemeralKey cifra a pubkey IN-PLACE: kp.Pub ja' vem
	// cifrada. Isso e' comportamento do upstream, preservado — o par so' e'
	// usado pela chave privada dali em diante.
	if got := ephemeralKey[codeIVEnd:codeWrappedKeyEnd]; string(got) != string(kp.Pub[:]) {
		t.Error("a pubkey no blob deveria ser a mesma (cifrada in-place) do par devolvido")
	}
}
