package group

import (
	"context"
	"errors"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// SetMemberAddMode e' a unica funcao de settings.go com logica
// antes da rede: a validacao do modo. Os modos validos nao dao para exercitar
// aqui (caem em sendIQ), mas o rejeitado retorna antes de tocar o socket.
func TestSetMemberAddModeRejectsInvalidMode(t *testing.T) {
	tr := newFakeTransport()
	err := SetMemberAddMode(context.Background(), tr, groupTestJID, types.GroupMemberAddMode("qualquer_coisa"))
	if err == nil {
		t.Fatal("esperado erro para modo invalido")
	}
	for _, want := range []types.GroupMemberAddMode{types.GroupMemberAddModeAdmin, types.GroupMemberAddModeAllMember} {
		if !strings.Contains(err.Error(), string(want)) {
			t.Errorf("mensagem de erro %q nao cita o modo valido %q", err, want)
		}
	}
}

// --- SetPhoto ---

func TestSetPhotoUploadsAndReturnsID(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
		Tag:   "picture",
		Attrs: waBinary.Attrs{"id": "PIC1"},
	}}}}
	got, err := SetPhoto(context.Background(), tr, groupTestJID, []byte("JPEG"))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != "PIC1" {
		t.Errorf("id = %q", got)
	}
	// Unico <iq> deste dominio fora do namespace w:g2: vai para o servidor com
	// o grupo em Target.
	sent := tr.sent[0]
	if sent.Namespace != pictureIQNamespace || sent.To != types.ServerJID || sent.Target != groupTestJID {
		t.Errorf("iq = %+v", sent)
	}
	nodes := sent.Content.([]waBinary.Node)
	if nodes[0].Attrs["type"] != "image" || string(nodes[0].Content.([]byte)) != "JPEG" {
		t.Errorf("conteudo = %+v", nodes[0])
	}
}

// avatar nil remove a foto: o conteudo vai vazio e o "ID" e' o sentinela.
func TestSetPhotoRemove(t *testing.T) {
	tr := newFakeTransport()
	got, err := SetPhoto(context.Background(), tr, groupTestJID, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != photoRemovedID {
		t.Errorf("id = %q, esperado %q", got, photoRemovedID)
	}
	if tr.sent[0].Content != nil {
		t.Errorf("Content = %+v, esperado nil na remocao", tr.sent[0].Content)
	}
}

func TestSetPhotoMapsNotAcceptableToInvalidImage(t *testing.T) {
	tr := newFakeTransport()
	tr.err = []error{testIQErrors.NotAcceptable}
	if _, err := SetPhoto(context.Background(), tr, groupTestJID, []byte("x")); !errors.Is(err, ErrInvalidImageFormat) {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	sentinel := errors.New("500")
	tr2.err = []error{sentinel}
	if _, err := SetPhoto(context.Background(), tr2, groupTestJID, []byte("x")); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestSetPhotoWithoutPictureIDInResponse(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{Tag: "picture"}}}}
	if _, err := SetPhoto(context.Background(), tr, groupTestJID, []byte("x")); err == nil {
		t.Fatal("esperado erro sem id na resposta")
	}
}

// --- SetName / SetDescription ---

func TestSetName(t *testing.T) {
	tr := newFakeTransport()
	if err := SetName(context.Background(), tr, groupTestJID, "Novo nome"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := sentContent(t, tr, 0)
	if node.Tag != subjectTag || string(node.Content.([]byte)) != "Novo nome" {
		t.Errorf("no = %s", node.XMLString())
	}
}

func TestSetDescription(t *testing.T) {
	tr := newFakeTransport()
	if err := SetDescription(context.Background(), tr, groupTestJID, "desc"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := sentContent(t, tr, 0)
	body := node.GetChildByTag(descriptionBodyTag)
	if node.Tag != descriptionTag || string(body.Content.([]byte)) != "desc" {
		t.Errorf("no = %s", node.XMLString())
	}
}

// --- SetTopic ---

func TestSetTopicWithBothIDs(t *testing.T) {
	tr := newFakeTransport()
	if err := SetTopic(context.Background(), tr, groupTestJID, "PREV", "NEW", "assunto"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(tr.sent) != 1 {
		t.Fatalf("iqs = %d, com previousID dado nao deve consultar o grupo", len(tr.sent))
	}
	node := sentContent(t, tr, 0)
	if node.Attrs["id"] != "NEW" || node.Attrs["prev"] != "PREV" {
		t.Errorf("attrs = %+v", node.Attrs)
	}
	body := node.GetChildByTag(descriptionBodyTag)
	if string(body.Content.([]byte)) != "assunto" {
		t.Errorf("body = %+v", body.Content)
	}
}

// Sem previousID, SetTopic busca o grupo antes — e' o unico ponto de
// settings.go que fala com GetInfo.
func TestSetTopicFetchesPreviousID(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith(waBinary.Node{
		Tag:   descriptionTag,
		Attrs: waBinary.Attrs{"id": "TOPICO_ANTIGO", "t": "1"},
		Content: []waBinary.Node{{
			Tag:     descriptionBodyTag,
			Content: []byte("antigo"),
		}},
	})}}}
	if err := SetTopic(context.Background(), tr, groupTestJID, "", "NEW", "novo"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(tr.sent) != 2 {
		t.Fatalf("iqs = %d, esperado consulta + update", len(tr.sent))
	}
	if got := sentContent(t, tr, 1).Attrs["prev"]; got != "TOPICO_ANTIGO" {
		t.Errorf("prev = %v", got)
	}
}

func TestSetTopicFetchFailure(t *testing.T) {
	tr := newFakeTransport()
	tr.err = []error{testIQErrors.NotFound}
	err := SetTopic(context.Background(), tr, groupTestJID, "", "NEW", "novo")
	if err == nil {
		t.Fatal("esperado erro ao buscar o grupo")
	}
	if len(tr.sent) != 1 {
		t.Errorf("iqs = %d, nao deveria ter enviado o update", len(tr.sent))
	}
}

// Topico vazio e' delecao: sem <body>, com delete="true".
func TestSetTopicEmptyIsDelete(t *testing.T) {
	tr := newFakeTransport()
	tr.msgID = "IDGERADO"
	if err := SetTopic(context.Background(), tr, groupTestJID, "PREV", "", ""); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := sentContent(t, tr, 0)
	if node.Attrs["delete"] != "true" {
		t.Errorf("attrs = %+v", node.Attrs)
	}
	// content vira um []Node nil: a interface nao e' nil, mas nao ha' <body>.
	if len(node.GetChildren()) != 0 {
		t.Errorf("Content = %+v, esperado sem filhos na delecao", node.Content)
	}
	// newID vazio: gerado por GenerateMessageID.
	if node.Attrs["id"] != "IDGERADO" {
		t.Errorf("id = %v", node.Attrs["id"])
	}
}

// previousID vazio E encontrado vazio: nenhum atributo prev e' escrito.
func TestSetTopicOmitsPrevWhenUnknown(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	if err := SetTopic(context.Background(), tr, groupTestJID, "", "NEW", "novo"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := sentContent(t, tr, 1).Attrs["prev"]; ok {
		t.Error("prev nao deveria estar presente")
	}
}

// --- flags booleanas ---

func TestSetLockedAndAnnounceTags(t *testing.T) {
	cases := []struct {
		name string
		call func(tr *fakeTransport) error
		want string
	}{
		{"locked", func(tr *fakeTransport) error {
			return SetLocked(context.Background(), tr, groupTestJID, true)
		}, lockedTag},
		{"unlocked", func(tr *fakeTransport) error {
			return SetLocked(context.Background(), tr, groupTestJID, false)
		}, unlockedTag},
		{"announce", func(tr *fakeTransport) error {
			return SetAnnounce(context.Background(), tr, groupTestJID, true)
		}, announcementTag},
		{"not_announce", func(tr *fakeTransport) error {
			return SetAnnounce(context.Background(), tr, groupTestJID, false)
		}, notAnnouncementTag},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakeTransport()
			if err := tc.call(tr); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got := sentContent(t, tr, 0).Tag; got != tc.want {
				t.Errorf("tag = %q, esperado %q", got, tc.want)
			}
		})
	}
}

func TestSetJoinApprovalMode(t *testing.T) {
	for _, tc := range []struct {
		mode bool
		want string
	}{{true, joinStateOn}, {false, joinStateOff}} {
		tr := newFakeTransport()
		if err := SetJoinApprovalMode(context.Background(), tr, groupTestJID, tc.mode); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		node := sentContent(t, tr, 0)
		join := node.GetChildByTag(joinTag)
		if node.Tag != membershipApprovalModeTag || join.Attrs["state"] != tc.want {
			t.Errorf("mode=%v: no = %s", tc.mode, node.XMLString())
		}
	}
}

func TestSetMemberAddModeAcceptsValidModes(t *testing.T) {
	for _, mode := range []types.GroupMemberAddMode{
		types.GroupMemberAddModeAdmin,
		types.GroupMemberAddModeAllMember,
	} {
		tr := newFakeTransport()
		if err := SetMemberAddMode(context.Background(), tr, groupTestJID, mode); err != nil {
			t.Fatalf("mode=%q: erro inesperado: %v", mode, err)
		}
		node := sentContent(t, tr, 0)
		if node.Tag != memberAddModeTag || string(node.Content.([]byte)) != string(mode) {
			t.Errorf("no = %s", node.XMLString())
		}
	}
}

// Erro de rede propaga em todos os setters simples.
func TestSettersPropagateIQError(t *testing.T) {
	sentinel := errors.New("nope")
	calls := map[string]func(tr *fakeTransport) error{
		"SetName":        func(tr *fakeTransport) error { return SetName(context.Background(), tr, groupTestJID, "x") },
		"SetDescription": func(tr *fakeTransport) error { return SetDescription(context.Background(), tr, groupTestJID, "x") },
		"SetLocked":      func(tr *fakeTransport) error { return SetLocked(context.Background(), tr, groupTestJID, true) },
		"SetAnnounce":    func(tr *fakeTransport) error { return SetAnnounce(context.Background(), tr, groupTestJID, true) },
		"SetJoinApprovalMode": func(tr *fakeTransport) error {
			return SetJoinApprovalMode(context.Background(), tr, groupTestJID, true)
		},
		"SetMemberAddMode": func(tr *fakeTransport) error {
			return SetMemberAddMode(context.Background(), tr, groupTestJID, types.GroupMemberAddModeAdmin)
		},
		"SetTopic": func(tr *fakeTransport) error {
			return SetTopic(context.Background(), tr, groupTestJID, "P", "N", "t")
		},
	}
	for name, call := range calls {
		tr := newFakeTransport()
		tr.err = []error{sentinel}
		if err := call(tr); !errors.Is(err, sentinel) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
