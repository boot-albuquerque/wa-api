package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
)

// Cobre o corte a hard para as quatro rotas ainda concatenadas (worktree
// http-dto-paths, CAP-10-paths): POST /group/inviteinfo, POST
// /group/invitelink, POST /chat/markread, POST /chat/send/pollvote. As
// quatro passaram a levar o identificador no CAMINHO
// (group_jid/invite_code/chat_jid/poll_message_id) em vez de no corpo — ver
// pkg/bootstrap/wiring_routes.go.
//
// Cada teste abaixo:
//  1. monta a rota EXACTAMENTE como wiring_routes.go a monta —
//     customhttp.InjectPathParams em cima do handler, no mux real — e prova
//     que o identificador do CAMINHO chega ao use case mesmo com o corpo sem
//     ele (ARMADILHAS #2: rota pela rota registada, não pelo handler cru);
//  2. tem um controlo negativo: a MESMA requisição contra uma rota que NÃO
//     envolve o handler em InjectPathParams falha com a recusa de campo em
//     falta — provando que o teste morde se o corte for desfeito.
func withPathParamInjection(next http.Handler) http.Handler {
	return customhttp.InjectPathParams(next)
}

func TestGetGroupInviteLink_CaminhoDoGroupJIDChegaAoUseCase(t *testing.T) {
	const groupJID = "120363411669320145@g.us"

	f := newGrpFakes()
	var recebeu domain.JID
	f.jids.ResolveJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		recebeu = domain.JID(raw)
		return domain.JID(raw), nil
	}
	f.directory.GetGroupInviteLinkFunc = func(context.Context, string, domain.JID) (string, error) {
		return "https://chat.whatsapp.com/abc", nil
	}
	uc := group.NewGetGroupInviteLinkUseCase(f.directory, f.jids, f.logger)
	handler := NewGetGroupInviteLinkHandler(uc)

	build := func(wrap bool) *mux.Router {
		registry := customhttp.NewHandlerRegistry()
		h := http.Handler(withContractUser(handler))
		if wrap {
			h = withPathParamInjection(h)
		}
		registry.Register("/groups/{group_jid}/invite-link", h, http.MethodGet)
		router := mux.NewRouter()
		registry.Apply(router)
		return router
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/groups/"+groupJID+"/invite-link", nil)
	build(true).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("com InjectPathParams: status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	if recebeu != groupJID {
		t.Fatalf("use case recebeu groupJID=%q, quero %q (do caminho)", recebeu, groupJID)
	}

	// Controlo negativo: SEM InjectPathParams, o corpo continua vazio e o
	// group_jid nunca chega ao use case.
	recNeg := httptest.NewRecorder()
	reqNeg := httptest.NewRequest(http.MethodGet, "/groups/"+groupJID+"/invite-link", nil)
	build(false).ServeHTTP(recNeg, reqNeg)
	if recNeg.Code == http.StatusOK {
		t.Fatalf("controlo negativo: sem InjectPathParams o pedido devia falhar (missing_group_jid), mas devolveu 200")
	}
}

func TestGetGroupInviteInfo_CaminhoDoInviteCodeChegaAoUseCase(t *testing.T) {
	const code = "IVccRoDVSbpHKNkgNxyZx4"

	f := newGrpFakes()
	var recebeu string
	f.directory.GetGroupInfoFromLinkFunc = func(_ context.Context, _ string, c string) (any, error) {
		recebeu = c
		return map[string]any{"JID": "120363411669320145@g.us"}, nil
	}
	uc := group.NewGetGroupInviteInfoUseCase(f.directory, f.logger)
	handler := NewGetGroupInviteInfoHandler(uc)

	build := func(wrap bool) *mux.Router {
		registry := customhttp.NewHandlerRegistry()
		h := http.Handler(withContractUser(handler))
		if wrap {
			h = withPathParamInjection(h)
		}
		registry.Register("/groups/invite-links/{invite_code}", h, http.MethodGet)
		router := mux.NewRouter()
		registry.Apply(router)
		return router
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/groups/invite-links/"+code, nil)
	build(true).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("com InjectPathParams: status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	if recebeu != code {
		t.Fatalf("use case recebeu Code=%q, quero %q (do caminho)", recebeu, code)
	}

	recNeg := httptest.NewRecorder()
	reqNeg := httptest.NewRequest(http.MethodGet, "/groups/invite-links/"+code, nil)
	build(false).ServeHTTP(recNeg, reqNeg)
	if recNeg.Code == http.StatusOK {
		t.Fatalf("controlo negativo: sem InjectPathParams o pedido devia falhar (missing_code), mas devolveu 200")
	}
}

func TestMarkRead_CaminhoDoChatJIDChegaAoUseCase(t *testing.T) {
	const chatJID = "554192421234@s.whatsapp.net"

	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}
	var recebeu domain.JID
	jr.ResolveJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		recebeu = domain.JID(raw)
		return domain.JID(raw), nil
	}
	cm.MarkReadFunc = func(context.Context, string, []string, time.Time, domain.JID, domain.JID) error {
		return nil
	}
	uc := message.NewMarkReadUseCase(cm, jr, silentLogger{})
	handler := NewMarkReadHandler(uc)

	build := func(wrap bool) *mux.Router {
		registry := customhttp.NewHandlerRegistry()
		h := http.Handler(withContractUser(handler))
		if wrap {
			h = withPathParamInjection(h)
		}
		registry.Register("/chats/{chat_jid}/read", h, http.MethodPost)
		router := mux.NewRouter()
		registry.Apply(router)
		return router
	}

	// Corpo sem ChatPhone: só Id, que a rota exige. ChatPhone tem de vir do
	// caminho.
	body := `{"Id":["3EB0A4B2AFD45E625C0917"]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chats/"+chatJID+"/read", strings.NewReader(body))
	build(true).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("com InjectPathParams: status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	if recebeu != chatJID {
		t.Fatalf("use case resolveu ChatPhone=%q, quero %q (do caminho)", recebeu, chatJID)
	}

	recNeg := httptest.NewRecorder()
	reqNeg := httptest.NewRequest(http.MethodPost, "/chats/"+chatJID+"/read", strings.NewReader(body))
	build(false).ServeHTTP(recNeg, reqNeg)
	if recNeg.Code == http.StatusOK {
		t.Fatalf("controlo negativo: sem InjectPathParams o pedido devia falhar (missing_chatphone), mas devolveu 200")
	}
}

func TestSendPollVote_CaminhoDoPollMessageIDChegaAoUseCase(t *testing.T) {
	const pollMessageID = "3EB0A4B2AFD45E625C0917"

	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}
	jr.ResolveJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		return domain.JID(raw), nil
	}
	var recebeuID string
	cm.SendPollVoteFunc = func(_ context.Context, _ string, _ domain.JID, payload domain.PollVotePayload, _ string) (domain.MessageSendResult, error) {
		recebeuID = payload.PollMessageID
		return domain.MessageSendResult{ID: "sent-id"}, nil
	}
	uc := message.NewSendPollVoteUseCase(cm, jr, silentLogger{})
	handler := NewSendPollVoteHandler(uc)

	build := func(wrap bool) *mux.Router {
		registry := customhttp.NewHandlerRegistry()
		h := http.Handler(withContractUser(handler))
		if wrap {
			h = withPathParamInjection(h)
		}
		registry.Register("/polls/{poll_message_id}/votes", h, http.MethodPost)
		router := mux.NewRouter()
		registry.Apply(router)
		return router
	}

	// Corpo sem PollMessageId: os outros três campos obrigatórios vêm no
	// corpo, o identificador da enquete tem de vir do caminho.
	body := `{"Phone":"120363411669320145@g.us","Sender":"5511999999999@s.whatsapp.net","PollMessageTimestamp":1755500100,"Options":["12h"]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/polls/"+pollMessageID+"/votes", strings.NewReader(body))
	build(true).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("com InjectPathParams: status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	if recebeuID != pollMessageID {
		t.Fatalf("use case recebeu PollMessageId=%q, quero %q (do caminho)", recebeuID, pollMessageID)
	}

	recNeg := httptest.NewRecorder()
	reqNeg := httptest.NewRequest(http.MethodPost, "/polls/"+pollMessageID+"/votes", strings.NewReader(body))
	build(false).ServeHTTP(recNeg, reqNeg)
	if recNeg.Code == http.StatusOK {
		t.Fatalf("controlo negativo: sem InjectPathParams o pedido devia falhar (missing_poll_message_id), mas devolveu 200")
	}
}
