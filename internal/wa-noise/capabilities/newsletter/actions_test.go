package newsletter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// A duracao vem em SEGUNDOS no atributo do <live_updates> da resposta.
func TestSubscribeLiveUpdatesLeADuracaoEmSegundos(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:   liveUpdatesTag,
			Attrs: waBinary.Attrs{liveUpdatesDurationAttr: "300"},
		}},
	}

	jid := testJID()
	dur, err := SubscribeLiveUpdates(context.Background(), f, jid)
	if err != nil {
		t.Fatalf("SubscribeLiveUpdates: %v", err)
	}
	if dur != 300*time.Second {
		t.Errorf("dur = %v, esperava 5m", dur)
	}
	iq := f.iqs[0]
	if iq.Namespace != Namespace || iq.Type != IQSet || iq.To != jid {
		t.Errorf("iq = %+v", iq)
	}
	if nodes := iq.Content.([]waBinary.Node); nodes[0].Tag != liveUpdatesTag {
		t.Errorf("tag = %q", nodes[0].Tag)
	}
}

// Resposta sem <live_updates>: GetChildByTag devolve o no zerado e a duracao
// vira zero, sem erro. Comportamento herdado do upstream.
func TestSubscribeLiveUpdatesSemNoDevolveZero(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{Tag: "iq"}

	dur, err := SubscribeLiveUpdates(context.Background(), f, testJID())
	if err != nil {
		t.Fatalf("SubscribeLiveUpdates: %v", err)
	}
	if dur != 0 {
		t.Errorf("dur = %v, esperava 0", dur)
	}
}

func TestSubscribeLiveUpdatesPropagaErro(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	dur, err := SubscribeLiveUpdates(context.Background(), f, testJID())
	if dur != 0 {
		t.Errorf("dur = %v, esperava 0", dur)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

func TestViewedItemsPreservaOrdemEIDs(t *testing.T) {
	items := viewedItems([]types.MessageServerID{7, 3, 11})
	if len(items) != 3 {
		t.Fatalf("esperava 3 itens, veio %d", len(items))
	}
	for i, want := range []types.MessageServerID{7, 3, 11} {
		if items[i].Tag != "item" {
			t.Errorf("item %d: tag = %q, esperava \"item\"", i, items[i].Tag)
		}
		got, ok := items[i].Attrs["server_id"].(types.MessageServerID)
		if !ok {
			t.Fatalf("item %d: server_id de tipo %T, esperava types.MessageServerID", i, items[i].Attrs["server_id"])
		}
		if got != want {
			t.Errorf("item %d: server_id = %d, esperava %d", i, got, want)
		}
	}
}

// Lista vazia continua produzindo um <list> sem filhos, nao nil: o servidor
// aceita o recibo vazio e o codigo nao deve tratar isso como erro.
func TestViewedItemsListaVazia(t *testing.T) {
	items := viewedItems(nil)
	if items == nil {
		t.Fatal("esperava slice vazio nao-nil")
	}
	if len(items) != 0 {
		t.Fatalf("esperava 0 itens, veio %d", len(items))
	}
}

// O canal de resposta e' registrado ANTES do envio; inverter a ordem abriria
// janela para a resposta chegar sem ouvinte.
func TestMarkViewedRegistraOCanalAntesDeEnviar(t *testing.T) {
	f := newFakeTransport()
	jid := testJID()

	if err := MarkViewed(context.Background(), f, jid, []types.MessageServerID{5}); err != nil {
		t.Fatalf("MarkViewed: %v", err)
	}
	if len(f.waitedFor) != 1 || f.waitedFor[0] != f.reqID {
		t.Errorf("waitResponse = %v, esperava [%s]", f.waitedFor, f.reqID)
	}
	if len(f.canceled) != 0 {
		t.Errorf("nao deveria ter cancelado, cancelou %v", f.canceled)
	}
	if len(f.nodes) != 1 {
		t.Fatalf("esperava 1 no enviado, veio %d", len(f.nodes))
	}
	node := f.nodes[0]
	if node.Tag != "receipt" || node.Attrs["type"] != "view" || node.Attrs["to"] != jid || node.Attrs["id"] != f.reqID {
		t.Errorf("no = %+v", node)
	}
	list := node.Content.([]waBinary.Node)
	if list[0].Tag != "list" {
		t.Errorf("filho = %+v", list[0])
	}
	if items := list[0].Content.([]waBinary.Node); len(items) != 1 {
		t.Errorf("itens = %v", items)
	}
}

// Falha no envio cancela o registro do canal — sem isso o mapa de respostas
// pendentes vazaria uma entrada por recibo perdido.
func TestMarkViewedCancelaORegistroSeOEnvioFalha(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("socket fechado")
	f.nodeErr = sentinel

	err := MarkViewed(context.Background(), f, testJID(), nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
	if len(f.canceled) != 1 || f.canceled[0] != f.reqID {
		t.Errorf("canceled = %v, esperava [%s]", f.canceled, f.reqID)
	}
}

func TestReactionAttrsComCodigo(t *testing.T) {
	jid := testJID()
	msgAttrs, rAttrs := reactionAttrs(jid, 42, "\U0001F600", "MSGID1")

	if msgAttrs["to"] != jid {
		t.Errorf("to = %v, esperava %v", msgAttrs["to"], jid)
	}
	if msgAttrs["id"] != types.MessageID("MSGID1") {
		t.Errorf("id = %v", msgAttrs["id"])
	}
	if msgAttrs["server_id"] != types.MessageServerID(42) {
		t.Errorf("server_id = %v", msgAttrs["server_id"])
	}
	if msgAttrs["type"] != "reaction" {
		t.Errorf("type = %v, esperava \"reaction\"", msgAttrs["type"])
	}
	if _, ok := msgAttrs["edit"]; ok {
		t.Error("reacao com codigo nao deveria carregar o atributo edit")
	}
	if rAttrs["code"] != "\U0001F600" {
		t.Errorf("code = %v", rAttrs["code"])
	}
}

// Reacao vazia significa REMOVER a reacao anterior. O fork nao manda code=""
// — manda uma revogacao do proprio remetente. Trocar isso faria o WhatsApp
// registrar uma reacao com codigo vazio em vez de apagar a existente.
func TestReactionAttrsVaziaViraRevogacao(t *testing.T) {
	jid := testJID()
	msgAttrs, rAttrs := reactionAttrs(jid, 42, "", "MSGID2")

	if len(rAttrs) != 0 {
		t.Errorf("reacao vazia nao deveria ter atributos, veio %v", rAttrs)
	}
	if msgAttrs["edit"] != string(types.EditAttributeSenderRevoke) {
		t.Errorf("edit = %v, esperava %v", msgAttrs["edit"], types.EditAttributeSenderRevoke)
	}
}

func TestSendReactionUsaOMessageIDDado(t *testing.T) {
	f := newFakeTransport()
	if err := SendReaction(context.Background(), f, testJID(), 42, "\U0001F600", "MEU-ID"); err != nil {
		t.Fatalf("SendReaction: %v", err)
	}
	node := f.nodes[0]
	if node.Tag != "message" || node.Attrs["id"] != types.MessageID("MEU-ID") {
		t.Errorf("no = %+v", node)
	}
	children := node.Content.([]waBinary.Node)
	if children[0].Tag != "reaction" || children[0].Attrs["code"] != "\U0001F600" {
		t.Errorf("filho = %+v", children[0])
	}
}

func TestSendReactionGeraOMessageIDQuandoVazio(t *testing.T) {
	f := newFakeTransport()
	if err := SendReaction(context.Background(), f, testJID(), 42, "x", ""); err != nil {
		t.Fatalf("SendReaction: %v", err)
	}
	if f.nodes[0].Attrs["id"] != f.msgID {
		t.Errorf("id = %v, esperava o gerado %q", f.nodes[0].Attrs["id"], f.msgID)
	}
}

func TestSendReactionPropagaErro(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.nodeErr = sentinel

	if err := SendReaction(context.Background(), f, testJID(), 1, "x", "ID"); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

func TestRespCreateNewsletterLeCampoXwa2NewsletterCreate(t *testing.T) {
	var resp respCreateNewsletter
	if err := json.Unmarshal([]byte(`{"xwa2_newsletter_create":{"id":"1234567890@newsletter"}}`), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Newsletter == nil {
		t.Fatal("xwa2_newsletter_create deveria ter sido decodificado")
	}
}

// CreateParams vai no corpo da mutation. Description e Picture sao omitempty;
// Name nao — o servidor exige o campo presente.
func TestCreateParamsOmiteOpcionais(t *testing.T) {
	b, err := json.Marshal(CreateParams{Name: "Canal"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"name":"Canal"}` {
		t.Errorf("payload = %s", b)
	}
}

func TestCreateSucesso(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter_create":{"id":"1234567890@newsletter"}}}`)

	meta, err := Create(context.Background(), f, CreateParams{Name: "Canal"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if meta == nil || meta.ID.User != "1234567890" {
		t.Fatalf("meta = %+v", meta)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	if !strings.Contains(string(nodes[0].Content.([]byte)), `"newsletter_input":{"name":"Canal"}`) {
		t.Errorf("payload = %s", nodes[0].Content)
	}
}

func TestCreatePropagaErroDoMex(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	meta, err := Create(context.Background(), f, CreateParams{Name: "Canal"})
	if meta != nil {
		t.Errorf("meta = %+v, esperava nil", meta)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

// Diferente de GetInfo, Create aborta no erro de unmarshal em vez de devolver
// o que conseguiu decodificar.
func TestCreateErroDeUnmarshal(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter_create":7}}`)

	if _, err := Create(context.Background(), f, CreateParams{Name: "Canal"}); err == nil {
		t.Fatal("esperava erro de unmarshal")
	}
}

func TestAcceptTOSNotice(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{Tag: "iq"}

	if err := AcceptTOSNotice(context.Background(), f, "20601218", "5"); err != nil {
		t.Fatalf("AcceptTOSNotice: %v", err)
	}
	iq := f.iqs[0]
	if iq.Namespace != "tos" || iq.Type != IQSet || iq.To != types.ServerJID {
		t.Errorf("iq = %+v", iq)
	}
	nodes := iq.Content.([]waBinary.Node)
	if nodes[0].Tag != "notice" || nodes[0].Attrs["id"] != "20601218" || nodes[0].Attrs["stage"] != "5" {
		t.Errorf("no = %+v", nodes[0])
	}
}

func TestAcceptTOSNoticePropagaErro(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if err := AcceptTOSNotice(context.Background(), f, "1", "2"); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

// mutationQueryID extrai a query ID do unico IQ registrado no duble.
func mutationQueryID(t *testing.T, f *fakeTransport) string {
	t.Helper()
	if len(f.iqs) != 1 {
		t.Fatalf("esperava 1 IQ, veio %d", len(f.iqs))
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	id, _ := nodes[0].Attrs[mexQueryIDAttr].(string)
	return id
}

// Silenciar e dessilenciar sao mutations DIFERENTES; trocar as duas inverteria
// o efeito de NewsletterToggleMute.
func TestToggleMuteEscolheAMutation(t *testing.T) {
	for _, tt := range []struct {
		mute bool
		want string
	}{
		{true, mutationMuteNewsletter},
		{false, mutationUnmuteNewsletter},
	} {
		f := newFakeTransport()
		f.iqResp = mexJSON(`{"data":{}}`)
		if err := ToggleMute(context.Background(), f, testJID(), tt.mute); err != nil {
			t.Fatalf("ToggleMute(%v): %v", tt.mute, err)
		}
		if got := mutationQueryID(t, f); got != tt.want {
			t.Errorf("ToggleMute(%v) usou %q, esperava %q", tt.mute, got, tt.want)
		}
	}
}

func TestToggleMutePropagaErro(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if err := ToggleMute(context.Background(), f, testJID(), true); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

// Seguir e deixar de seguir mandam o JID como string em "newsletter_id" e usam
// mutations distintas.
func TestFollowEUnfollowUsamMutationsDistintas(t *testing.T) {
	jid := testJID()

	fFollow := newFakeTransport()
	fFollow.iqResp = mexJSON(`{"data":{}}`)
	if err := Follow(context.Background(), fFollow, jid); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if got := mutationQueryID(t, fFollow); got != mutationFollowNewsletter {
		t.Errorf("Follow usou %q, esperava %q", got, mutationFollowNewsletter)
	}
	nodes := fFollow.iqs[0].Content.([]waBinary.Node)
	if !strings.Contains(string(nodes[0].Content.([]byte)), `"newsletter_id":"`+jid.String()+`"`) {
		t.Errorf("payload = %s", nodes[0].Content)
	}

	fUnfollow := newFakeTransport()
	fUnfollow.iqResp = mexJSON(`{"data":{}}`)
	if err := Unfollow(context.Background(), fUnfollow, jid); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	if got := mutationQueryID(t, fUnfollow); got != mutationUnfollowNewsletter {
		t.Errorf("Unfollow usou %q, esperava %q", got, mutationUnfollowNewsletter)
	}
}

func TestFollowEUnfollowPropagamErro(t *testing.T) {
	sentinel := errors.New("boom")

	fFollow := newFakeTransport()
	fFollow.iqErr = sentinel
	if err := Follow(context.Background(), fFollow, testJID()); !errors.Is(err, sentinel) {
		t.Fatalf("Follow: err = %v, esperava %v", err, sentinel)
	}

	fUnfollow := newFakeTransport()
	fUnfollow.iqErr = sentinel
	if err := Unfollow(context.Background(), fUnfollow, testJID()); !errors.Is(err, sentinel) {
		t.Fatalf("Unfollow: err = %v, esperava %v", err, sentinel)
	}
}

// ---------------------------------------------------------------------------
// F233b — DemoteAdmin, ChangeOwner, Delete
// ---------------------------------------------------------------------------

func testUserJID() types.JID {
	return types.NewJID("5516900000000", types.DefaultUserServer)
}

func TestDemoteAdminSendsCorrectMutation(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{}}`)
	channelJID := testJID()
	userJID := testUserJID()

	if err := DemoteAdmin(context.Background(), f, channelJID, userJID); err != nil {
		t.Fatalf("DemoteAdmin: %v", err)
	}
	if got := mutationQueryID(t, f); got != mutationDemoteAdmin {
		t.Errorf("DemoteAdmin used query ID %q, want %q", got, mutationDemoteAdmin)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	payload := string(nodes[0].Content.([]byte))
	if !strings.Contains(payload, `"newsletter_id":"`+channelJID.String()+`"`) {
		t.Errorf("payload missing newsletter_id: %s", payload)
	}
	if !strings.Contains(payload, `"user_id":"`+userJID.String()+`"`) {
		t.Errorf("payload missing user_id: %s", payload)
	}
}

func TestDemoteAdminPropagatesError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if err := DemoteAdmin(context.Background(), f, testJID(), testUserJID()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestChangeOwnerSendsCorrectMutation(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{}}`)
	channelJID := testJID()
	newOwner := testUserJID()

	if err := ChangeOwner(context.Background(), f, channelJID, newOwner); err != nil {
		t.Fatalf("ChangeOwner: %v", err)
	}
	if got := mutationQueryID(t, f); got != mutationChangeOwner {
		t.Errorf("ChangeOwner used query ID %q, want %q", got, mutationChangeOwner)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	payload := string(nodes[0].Content.([]byte))
	if !strings.Contains(payload, `"newsletter_id":"`+channelJID.String()+`"`) {
		t.Errorf("payload missing newsletter_id: %s", payload)
	}
	if !strings.Contains(payload, `"user_id":"`+newOwner.String()+`"`) {
		t.Errorf("payload missing user_id: %s", payload)
	}
}

func TestChangeOwnerPropagatesError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if err := ChangeOwner(context.Background(), f, testJID(), testUserJID()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestDeleteSendsCorrectMutation(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{}}`)
	channelJID := testJID()

	if err := Delete(context.Background(), f, channelJID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := mutationQueryID(t, f); got != mutationDeleteNewsletter {
		t.Errorf("Delete used query ID %q, want %q", got, mutationDeleteNewsletter)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	payload := string(nodes[0].Content.([]byte))
	if !strings.Contains(payload, `"newsletter_id":"`+channelJID.String()+`"`) {
		t.Errorf("payload missing newsletter_id: %s", payload)
	}
}

func TestDeletePropagatesError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if err := Delete(context.Background(), f, testJID()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// ---------------------------------------------------------------------------
// F233(b) — CreateAdminInvite, AcceptAdminInvite, RevokeAdminInvite
// ---------------------------------------------------------------------------

func TestCreateAdminInviteSendsCorrectMutation(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{}}`)
	channelJID := testJID()
	userJID := testUserJID()

	if _, err := CreateAdminInvite(context.Background(), f, channelJID, userJID); err != nil {
		t.Fatalf("CreateAdminInvite: %v", err)
	}
	if got := mutationQueryID(t, f); got != mutationCreateAdminInvite {
		t.Errorf("CreateAdminInvite used query ID %q, want %q", got, mutationCreateAdminInvite)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	payload := string(nodes[0].Content.([]byte))
	if !strings.Contains(payload, `"newsletter_id":"`+channelJID.String()+`"`) {
		t.Errorf("payload missing newsletter_id: %s", payload)
	}
	if !strings.Contains(payload, `"user_id":"`+userJID.String()+`"`) {
		t.Errorf("payload missing user_id: %s", payload)
	}
}

// TestCreateAdminInviteDevolveOPayloadCru trava a F261: o servidor confirma
// o id e a expiração do convite (`invite_expiration_time`, medido
// 2026-08-26, epoch Unix em segundos como STRING), e até esta correção esse
// valor era lido e descartado — a rota HTTP respondia `data:null`.
func TestCreateAdminInviteDevolveOPayloadCru(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter_admin_invite_create":{"id":"120363411706831441@newsletter","invite_expiration_time":"1788351063"}}}`)

	invite, err := CreateAdminInvite(context.Background(), f, testJID(), testUserJID())
	if err != nil {
		t.Fatalf("CreateAdminInvite: %v", err)
	}
	if invite.ID != "120363411706831441@newsletter" {
		t.Errorf("invite.ID = %q, queria o id devolvido pelo servidor", invite.ID)
	}
	want := time.Unix(1788351063, 0).UTC()
	if !invite.ExpirationTime.Equal(want) {
		t.Errorf("invite.ExpirationTime = %v, queria %v (epoch 1788351063)", invite.ExpirationTime, want)
	}
}

func TestCreateAdminInvitePropagatesError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if _, err := CreateAdminInvite(context.Background(), f, testJID(), testUserJID()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestAcceptAdminInviteSendsCorrectMutation(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{}}`)
	channelJID := testJID()

	if err := AcceptAdminInvite(context.Background(), f, channelJID); err != nil {
		t.Fatalf("AcceptAdminInvite: %v", err)
	}
	if got := mutationQueryID(t, f); got != mutationAcceptAdminInvite {
		t.Errorf("AcceptAdminInvite used query ID %q, want %q", got, mutationAcceptAdminInvite)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	payload := string(nodes[0].Content.([]byte))
	if !strings.Contains(payload, `"newsletter_id":"`+channelJID.String()+`"`) {
		t.Errorf("payload missing newsletter_id: %s", payload)
	}
	if strings.Contains(payload, `"user_id"`) {
		t.Errorf("accept must NOT send user_id, payload: %s", payload)
	}
}

func TestAcceptAdminInvitePropagatesError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if err := AcceptAdminInvite(context.Background(), f, testJID()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestRevokeAdminInviteSendsCorrectMutation(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{}}`)
	channelJID := testJID()
	userJID := testUserJID()

	if err := RevokeAdminInvite(context.Background(), f, channelJID, userJID); err != nil {
		t.Fatalf("RevokeAdminInvite: %v", err)
	}
	if got := mutationQueryID(t, f); got != mutationRevokeAdminInvite {
		t.Errorf("RevokeAdminInvite used query ID %q, want %q", got, mutationRevokeAdminInvite)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	payload := string(nodes[0].Content.([]byte))
	if !strings.Contains(payload, `"newsletter_id":"`+channelJID.String()+`"`) {
		t.Errorf("payload missing newsletter_id: %s", payload)
	}
	if !strings.Contains(payload, `"user_id":"`+userJID.String()+`"`) {
		t.Errorf("payload missing user_id: %s", payload)
	}
}

func TestRevokeAdminInvitePropagatesError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if err := RevokeAdminInvite(context.Background(), f, testJID(), testUserJID()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}
