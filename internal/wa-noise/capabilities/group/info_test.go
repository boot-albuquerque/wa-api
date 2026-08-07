package group

import (
	"context"
	"errors"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// --- sendIQ / SendIQ ---

func TestSendIQBuildsGroupNamespaceQuery(t *testing.T) {
	tr := newFakeTransport()
	content := waBinary.Node{Tag: "qualquer"}
	if _, err := SendIQ(context.Background(), tr, IQSet, groupTestJID, content); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(tr.sent) != 1 {
		t.Fatalf("iqs enviados = %d", len(tr.sent))
	}
	got := tr.sent[0]
	if got.Namespace != IQNamespace || got.Type != IQSet || got.To != groupTestJID {
		t.Errorf("iq = %+v", got)
	}
	// O conteudo vai sempre embrulhado em []Node de um elemento.
	nodes, ok := got.Content.([]waBinary.Node)
	if !ok || len(nodes) != 1 || nodes[0].Tag != "qualquer" {
		t.Errorf("Content = %+v", got.Content)
	}
	if !got.Target.IsEmpty() {
		t.Errorf("Target = %s, os iq de w:g2 nunca o usam", got.Target)
	}
}

func TestSendIQPropagatesError(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("boom")
	tr.err = []error{sentinel}
	if _, err := SendIQ(context.Background(), tr, IQGet, groupTestJID, waBinary.Node{}); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

// --- GetJoined ---

func TestGetJoinedParsesCachesAndPersists(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	tr.resp = []*waBinary.Node{{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag: "groups",
			Content: []waBinary.Node{
				groupNodeWith(participantNode(groupTestLIDJID, waBinary.Attrs{
					"phone_number": groupTestPNJID,
					"display_name": "5511XXXX999",
				})),
				{Tag: "lixo_inesperado"},
			},
		}},
	}}
	infos, err := GetJoined(context.Background(), tr)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(infos) != 1 || infos[0].JID != groupTestJID {
		t.Fatalf("infos = %+v", infos)
	}
	if _, ok := tr.cached(groupTestJID); !ok {
		t.Error("grupo da lista deveria ter entrado no cache")
	}
	if len(st.lidPairs) != 1 || st.lidPairs[0].LID != groupTestLIDJID {
		t.Errorf("lidPairs persistidos = %+v", st.lidPairs)
	}
	if len(st.redacted) != 1 {
		t.Errorf("redacted persistidos = %+v", st.redacted)
	}
	// O IQ foi montado com <participating> pedindo participantes e descricao.
	nodes := tr.sent[0].Content.([]waBinary.Node)
	if nodes[0].Tag != "participating" || len(nodes[0].GetChildren()) != 2 {
		t.Errorf("no de consulta = %s", nodes[0].XMLString())
	}
}

// Um <group> que nao parseia e' logado e **mesmo assim** entra na lista: era o
// comportamento do upstream (parsed nunca e' nil junto com erro).
func TestGetJoinedKeepsUnparseableGroup(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{
		Content: []waBinary.Node{{
			Tag:     "groups",
			Content: []waBinary.Node{{Tag: nodeTag}}, // sem id/creation
		}},
	}}
	infos, err := GetJoined(context.Background(), tr)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(infos) != 1 {
		t.Errorf("infos = %+v", infos)
	}
}

func TestGetJoinedMissingGroupsElement(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetJoined(context.Background(), tr)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != "groups" {
		t.Fatalf("err = %v", err)
	}
}

func TestGetJoinedPropagatesIQError(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("rede caiu")
	tr.err = []error{sentinel}
	if _, err := GetJoined(context.Background(), tr); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

// Falha ao persistir e' logada, nao propagada: o chamador ainda recebe a lista.
func TestGetJoinedSurvivesStoreErrors(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.lidPutErr = errors.New("db off")
	st.redErr = errors.New("db off")
	tr.resp = []*waBinary.Node{{
		Content: []waBinary.Node{{Tag: "groups", Content: []waBinary.Node{groupNodeWith()}}},
	}}
	infos, err := GetJoined(context.Background(), tr)
	if err != nil {
		t.Fatalf("erro de store nao deveria vazar: %v", err)
	}
	if len(infos) != 1 {
		t.Errorf("infos = %+v", infos)
	}
}

// --- GetSubGroups ---

func TestGetSubGroupsParsesTargets(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{
		Content: []waBinary.Node{{
			Tag: "sub_groups",
			Content: []waBinary.Node{
				{Tag: nodeTag, Attrs: waBinary.Attrs{"jid": groupTestJID, "subject": "Sub"}},
				{Tag: "outra_tag"},
			},
		}},
	}}
	got, err := GetSubGroups(context.Background(), tr, groupTestJID)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 || got[0].JID != groupTestJID || got[0].Name != "Sub" {
		t.Errorf("subgrupos = %+v", got)
	}
}

func TestGetSubGroupsUnparseableChild(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{
		Content: []waBinary.Node{{
			Tag:     "sub_groups",
			Content: []waBinary.Node{{Tag: nodeTag}}, // sem jid nem id
		}},
	}}
	if _, err := GetSubGroups(context.Background(), tr, groupTestJID); err == nil {
		t.Fatal("esperado erro de parse")
	}
}

func TestGetSubGroupsMissingElementAndIQError(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetSubGroups(context.Background(), tr, groupTestJID)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != "sub_groups" {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	sentinel := errors.New("iq falhou")
	tr2.err = []error{sentinel}
	if _, err := GetSubGroups(context.Background(), tr2, groupTestJID); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

// --- GetLinkedParticipants ---

func TestGetLinkedParticipantsPersistsMappings(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	tr.resp = []*waBinary.Node{{
		Content: []waBinary.Node{{
			Tag: "linked_groups_participants",
			Content: []waBinary.Node{
				participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID}),
			},
		}},
	}}
	members, err := GetLinkedParticipants(context.Background(), tr, groupTestJID)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(members) != 1 || members[0] != groupTestLIDJID {
		t.Errorf("membros = %v", members)
	}
	if len(st.lidPairs) != 1 {
		t.Errorf("lidPairs = %+v", st.lidPairs)
	}
}

// Sem nenhum par a gravar, o store nao e' tocado (guarda `len(lidPairs) > 0`).
func TestGetLinkedParticipantsSkipsEmptyMappingWrite(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.lidPutErr = errors.New("se chamado, o teste ainda passa — mas nao deve ser")
	tr.resp = []*waBinary.Node{{
		Content: []waBinary.Node{{Tag: "linked_groups_participants"}},
	}}
	members, err := GetLinkedParticipants(context.Background(), tr, groupTestJID)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("membros = %v", members)
	}
	if len(st.lidPairs) != 0 {
		t.Errorf("gravou %+v sem nenhum par", st.lidPairs)
	}
}

func TestGetLinkedParticipantsStoreErrorIsLoggedNotReturned(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.lidPutErr = errors.New("db off")
	tr.resp = []*waBinary.Node{{
		Content: []waBinary.Node{{
			Tag:     "linked_groups_participants",
			Content: []waBinary.Node{participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID})},
		}},
	}}
	if _, err := GetLinkedParticipants(context.Background(), tr, groupTestJID); err != nil {
		t.Fatalf("erro de store nao deveria vazar: %v", err)
	}
}

func TestGetLinkedParticipantsMissingElementAndIQError(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetLinkedParticipants(context.Background(), tr, groupTestJID)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != "linked_groups_participants" {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	sentinel := errors.New("iq falhou")
	tr2.err = []error{sentinel}
	if _, err := GetLinkedParticipants(context.Background(), tr2, groupTestJID); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

// --- GetInfo: mapeamento de erro de IQ ---

func TestGetInfoMapsIQErrors(t *testing.T) {
	cases := []struct {
		name string
		iq   error
		want error
	}{
		{"404", testIQErrors.NotFound, ErrNotFound},
		{"403", testIQErrors.Forbidden, ErrNotInGroup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.err = []error{tc.iq}
			_, err := GetInfo(context.Background(), tr, groupTestJID, true)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, esperado %v", err, tc.want)
			}
			// O erro de IQ original continua alcancavel por Unwrap.
			if !errors.Is(err, tc.iq) {
				t.Errorf("err = %v perdeu o erro de IQ original", err)
			}
		})
	}
}

func TestGetInfoPropagatesOtherIQErrors(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("500")
	tr.err = []error{sentinel}
	_, err := GetInfo(context.Background(), tr, groupTestJID, true)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotInGroup) {
		t.Errorf("err = %v foi traduzido indevidamente", err)
	}
}

func TestGetInfoMissingGroupNode(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetInfo(context.Background(), tr, groupTestJID, true)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != "groups" {
		t.Fatalf("err = %v", err)
	}
}

// Erro de parse do <group> devolve o info parcial junto — os chamadores logam
// info.JID.
func TestGetInfoReturnsPartialInfoOnParseError(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{Tag: nodeTag}}}}
	info, err := GetInfo(context.Background(), tr, groupTestJID, true)
	if err == nil {
		t.Fatal("esperado erro de parse")
	}
	if info == nil {
		t.Fatal("info nil junto com erro")
	}
	if _, ok := tr.cached(groupTestJID); ok {
		t.Error("grupo que falhou o parse nao deve ser cacheado")
	}
}

func TestGetInfoCachesAndPersists(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.lidPutErr = errors.New("db off") // logado, nao propagado
	st.redErr = errors.New("db off")
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith(
		participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID, "display_name": "X"}),
	)}}}
	info, err := GetInfo(context.Background(), tr, groupTestJID, true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if info.JID != groupTestJID {
		t.Errorf("JID = %s", info.JID)
	}
	cached, ok := tr.cached(groupTestJID)
	if !ok || len(cached.Members) != 1 {
		t.Errorf("cache = %+v (ok=%v)", cached, ok)
	}
	if len(st.lidPairs) != 1 || len(st.redacted) != 1 {
		t.Errorf("persistido: lids=%+v redacted=%+v", st.lidPairs, st.redacted)
	}
}

// --- GetOrFetch ---

func TestGetOrFetchReturnsCachedWithoutQuerying(t *testing.T) {
	tr := newFakeTransport()
	want := &Meta{Members: []types.JID{groupTestPNJID}}
	tr.putCached(groupTestJID, want)
	got, err := GetOrFetch(context.Background(), tr, groupTestJID)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != want {
		t.Errorf("meta = %+v, esperado a entrada em cache", got)
	}
	if len(tr.sent) != 0 {
		t.Errorf("consultou o servidor com o cache quente: %+v", tr.sent)
	}
}

func TestGetOrFetchQueriesOnMiss(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith(
		participantNode(groupTestPNJID, nil),
	)}}}
	got, err := GetOrFetch(context.Background(), tr, groupTestJID)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got == nil || len(got.Members) != 1 {
		t.Fatalf("meta = %+v", got)
	}
	if len(tr.sent) != 1 {
		t.Errorf("iqs = %d, esperado exatamente 1", len(tr.sent))
	}
}

func TestGetOrFetchPropagatesQueryError(t *testing.T) {
	tr := newFakeTransport()
	tr.err = []error{testIQErrors.NotFound}
	got, err := GetOrFetch(context.Background(), tr, groupTestJID)
	if got != nil {
		t.Errorf("meta = %+v, queria nil", got)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// Quando o servidor ecoa um `id` diferente do consultado, a entrada e' gravada
// sob OUTRA chave e o metadado pedido nao aparece no cache.
//
// GetOrFetch devolvia (nil, nil) nesse caso — "sucesso sem resultado", forma
// que o compilador nao ajuda a tratar (F40). Agora devolve ErrNotFound, com o
// JID consultado na mensagem.
func TestGetOrFetchDevolveErroQuandoOServidorEcoaOutroID(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	other := types.NewJID("99999", types.GroupServer)
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
		Tag:   nodeTag,
		Attrs: waBinary.Attrs{"id": other.User, "creation": "1"},
	}}}}
	got, err := GetOrFetch(context.Background(), tr, groupTestJID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, esperava ErrNotFound", err)
	}
	if got != nil {
		t.Fatalf("meta = %+v, esperado nil", got)
	}
	if !strings.Contains(err.Error(), groupTestJID.String()) {
		t.Errorf("a mensagem deveria nomear o JID consultado: %v", err)
	}
	if _, ok := tr.cached(other); !ok {
		t.Error("a entrada deveria ter sido gravada sob o id ecoado")
	}
}

// --- Cache.Delete ---

func TestCacheDeleteRemovesEntry(t *testing.T) {
	tr := newFakeTransport()
	tr.putCached(groupTestJID, &Meta{})
	tr.cache.Delete(groupTestJID)
	if _, ok := tr.cached(groupTestJID); ok {
		t.Error("entrada deveria ter sido removida")
	}
	// Apagar de um cache nunca escrito (mapa nil) nao pode estourar.
	other := newFakeTransport()
	other.cache.Delete(groupTestJID)
}
