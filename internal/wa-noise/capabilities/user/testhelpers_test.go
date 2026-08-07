package user

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waVnameCert"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

var (
	userTestPNJID  = types.NewJID("5511999", types.DefaultUserServer)
	userTestLIDJID = types.NewJID("8877", types.HiddenUserServer)
	userTestFBJID  = types.NewJID("12345", types.MessengerServer)
	userTestPN2JID = types.NewJID("5511888", types.DefaultUserServer)
)

// testIQErrors espelha os sentinelas de IQ da raiz. Sao errors.New proprios de
// proposito: os testes deste pacote so' precisam que errors.Is case por
// identidade, e depender dos valores da raiz reintroduziria o import que a
// extracao removeu. O contrato de que a raiz entrega os ponteiros certos e'
// travado do outro lado, pelo `var _ user.Transport = userTransport{}` em
// user_transport.go.
var testIQErrors = IQErrors{
	NotAuthorized: errors.New("iq 401"),
	NotFound:      errors.New("iq 404"),
}

// testElementMissing espelha *whatsmeow.ElementMissingError.
type testElementMissing struct {
	Tag string
	In  string
}

func (e *testElementMissing) Error() string {
	return fmt.Sprintf("missing <%s> element in %s", e.Tag, e.In)
}

// testWrappedIQError espelha o *wrappedIQError da raiz: Is() casa contra o erro
// humano, Unwrap() devolve o erro de IQ.
type testWrappedIQError struct {
	human error
	iq    error
}

func (e *testWrappedIQError) Error() string { return e.human.Error() }
func (e *testWrappedIQError) Is(other error) bool {
	return errors.Is(other, e.human)
}
func (e *testWrappedIQError) Unwrap() error { return e.iq }

// fakeTransport e' o duble de user.Transport. Registra os <iq> enviados e os
// eventos publicados, e devolve respostas/erros programados, sem socket nem
// sessao Noise.
type fakeTransport struct {
	cache DeviceCache
	store *store.Device

	sent   []IQ
	events []any
	// resp e' consultado por indice de chamada; um nil deixa o zero.
	resp []*waBinary.Node
	err  []error

	reqID string
	// hash, quando nao vazio, e' o dhash devolvido para qualquer lista.
	hash string
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{reqID: "sid-1", store: &store.Device{}}
}

func (f *fakeTransport) SendIQ(_ context.Context, query IQ) (*waBinary.Node, error) {
	i := len(f.sent)
	f.sent = append(f.sent, query)
	var resp *waBinary.Node
	var err error
	if i < len(f.resp) {
		resp = f.resp[i]
	}
	if i < len(f.err) {
		err = f.err[i]
	}
	if resp == nil && err == nil {
		resp = &waBinary.Node{}
	}
	return resp, err
}

func (f *fakeTransport) Store() *store.Device      { return f.store }
func (f *fakeTransport) DeviceCache() *DeviceCache { return &f.cache }
func (f *fakeTransport) Log() waLog.Logger         { return waLog.Noop }
func (f *fakeTransport) GenerateRequestID() string { return f.reqID }
func (f *fakeTransport) DispatchEvent(evt any)     { f.events = append(f.events, evt) }

// ParticipantListHash e' deterministico e independente do algoritmo real
// (participantListHashV2 vive na raiz, no dominio de envio): o que os testes
// deste pacote observam e' que o valor calculado da lista chega ao cache.
func (f *fakeTransport) ParticipantListHash(jids []types.JID) string {
	if f.hash != "" {
		return f.hash
	}
	return fmt.Sprintf("h:%d", len(jids))
}

func (f *fakeTransport) ElementMissing(tag, in string) error {
	return &testElementMissing{Tag: tag, In: in}
}

func (f *fakeTransport) WrapIQError(human, iq error) error {
	return &testWrappedIQError{human: human, iq: iq}
}

func (f *fakeTransport) IQErrors() IQErrors { return testIQErrors }

var _ Transport = (*fakeTransport)(nil)

// cached devolve a entrada de jid no cache do duble, tomando o lock.
func (f *fakeTransport) cached(jid types.JID) (DeviceEntry, bool) {
	f.cache.Lock()
	defer f.cache.Unlock()
	return f.cache.GetLocked(jid)
}

// putCached grava uma entrada no cache do duble, tomando o lock.
func (f *fakeTransport) putCached(jid types.JID, entry DeviceEntry) {
	f.cache.Lock()
	defer f.cache.Unlock()
	f.cache.SetLocked(jid, entry)
}

// --- dubles de store ---

// fakeStores implementa de uma vez as tres fatias de store.Device que este
// dominio toca (Contacts, LIDs, PrivacyTokens), registrando o que foi gravado.
// Embute store.NoopStore para satisfazer o resto de cada interface: qualquer
// metodo nao previsto devolve erro em vez de compilar por acidente.
type fakeStores struct {
	*store.NoopStore

	lidPairs  []store.LIDMapping
	lidPutErr error

	pushNames     map[types.JID]string
	businessNames map[types.JID]string
	// putErr, quando nao nil, e' devolvido por PutPushName/PutBusinessName.
	putErr error
	// noChange faz as duas gravacoes reportarem "nao mudou".
	noChange bool

	altJID    types.JID
	altJIDErr error

	token    *store.PrivacyToken
	tokenErr error
}

func newFakeStores() *fakeStores {
	return &fakeStores{
		NoopStore:     &store.NoopStore{Error: errors.New("nao previsto neste teste")},
		pushNames:     map[types.JID]string{},
		businessNames: map[types.JID]string{},
	}
}

func (f *fakeStores) PutPushName(_ context.Context, jid types.JID, name string) (bool, string, error) {
	if f.putErr != nil {
		return false, "", f.putErr
	}
	previous := f.pushNames[jid]
	f.pushNames[jid] = name
	return !f.noChange, previous, nil
}

func (f *fakeStores) PutBusinessName(_ context.Context, jid types.JID, name string) (bool, string, error) {
	if f.putErr != nil {
		return false, "", f.putErr
	}
	previous := f.businessNames[jid]
	f.businessNames[jid] = name
	return !f.noChange, previous, nil
}

func (f *fakeStores) PutManyLIDMappings(_ context.Context, m []store.LIDMapping) error {
	f.lidPairs = append(f.lidPairs, m...)
	return f.lidPutErr
}

func (f *fakeStores) GetLIDForPN(context.Context, types.JID) (types.JID, error) {
	return f.altJID, f.altJIDErr
}

func (f *fakeStores) GetPNForLID(context.Context, types.JID) (types.JID, error) {
	return f.altJID, f.altJIDErr
}

func (f *fakeStores) GetPrivacyToken(context.Context, types.JID) (*store.PrivacyToken, error) {
	return f.token, f.tokenErr
}

// withStores liga um fakeStores ao duble de transporte e o devolve.
func (f *fakeTransport) withStores() *fakeStores {
	st := newFakeStores()
	f.store = &store.Device{LIDs: st, Contacts: st, PrivacyTokens: st}
	return st
}

// --- montadores de no ---

// usyncResponse monta a resposta completa de um <iq> de usync:
// <usync><list><user .../>...</list></usync>.
func usyncResponse(users ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag: usyncNodeTag,
			Content: []waBinary.Node{{
				Tag:     usyncListTag,
				Content: users,
			}},
		}},
	}
}

// usyncUser monta um <user jid=...> com os filhos dados.
func usyncUser(jid types.JID, children ...waBinary.Node) waBinary.Node {
	return waBinary.Node{
		Tag:     usyncUserTag,
		Attrs:   waBinary.Attrs{"jid": jid},
		Content: children,
	}
}

// deviceNode monta um <device id=... [is_hosted=...]>.
func deviceNode(id string, hosted bool) waBinary.Node {
	attrs := waBinary.Attrs{"id": id}
	if hosted {
		attrs["is_hosted"] = "true"
	}
	return waBinary.Node{Tag: deviceNodeTag, Attrs: attrs}
}

// devicesNode monta o <devices><device-list>...</device-list></devices> que o
// servidor devolve dentro de cada <user> da resposta usync.
func devicesNode(children ...waBinary.Node) waBinary.Node {
	return waBinary.Node{
		Tag: devicesNodeTag,
		Content: []waBinary.Node{{
			Tag:     deviceListNodeTag,
			Content: children,
		}},
	}
}

// verifiedNameCertBytes serializa um certificado de nome verificado minimo.
func verifiedNameCertBytes(t *testing.T, name string) []byte {
	t.Helper()
	details, err := proto.Marshal(&waVnameCert.VerifiedNameCertificate_Details{
		VerifiedName: proto.String(name),
	})
	if err != nil {
		t.Fatalf("failed to marshal details: %v", err)
	}
	cert, err := proto.Marshal(&waVnameCert.VerifiedNameCertificate{Details: details})
	if err != nil {
		t.Fatalf("failed to marshal cert: %v", err)
	}
	return cert
}
