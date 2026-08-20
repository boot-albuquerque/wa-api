package chat_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
)

// Os três caminhos da fusão que a suíte de rota não alcança, porque exigem que
// a LEITURA da chave alternativa se comporte mal — e o fixture de rota usa um
// SQLite real, que se recusa a falhar quando lhe apetece.
//
// Não são testes de cobertura: cada um trava uma decisão de comportamento que,
// invertida, degrada a resposta em silêncio.

const lidUser = "U-1"

type readerStub struct {
	porChave map[string][]appport.ChatHistoryMessage
	falhaEm  map[string]error
	lidas    []string
}

func (r *readerStub) ListChatMessages(_ context.Context, _, chatJID string, _ int) ([]appport.ChatHistoryMessage, error) {
	r.lidas = append(r.lidas, chatJID)
	if err, ok := r.falhaEm[chatJID]; ok {
		return nil, err
	}
	return r.porChave[chatJID], nil
}

func (r *readerStub) ChatIndexByUser(context.Context, string) (map[string][]appport.ChatIndexEntry, error) {
	return nil, nil
}

// HistoryLimit devolve 50: o gate de histórico não é o que está sob teste aqui,
// e um 0 faria os testes morrerem em 501 antes de chegarem à fusão.
func (r *readerStub) HistoryLimit(context.Context, string) (int, error) { return 50, nil }

// resolverStub imita a REGRA REAL (adapters/user/adapter.go:119): mapeamento
// desconhecido é JID vazia com erro nil, e um identificador que não seja @lid é
// recusado pelo store.
type resolverStub struct {
	pn    string
	falha error
}

func (r resolverStub) GetPNForLID(_ context.Context, _ string, lid domain.JID) (domain.JID, error) {
	if r.falha != nil {
		return "", r.falha
	}
	if !strings.HasSuffix(string(lid), "@lid") {
		return "", errors.New("store recusa: nao e' @lid")
	}
	return domain.JID(r.pn), nil
}

func msg(id string, quando time.Time) appport.ChatHistoryMessage {
	return appport.ChatHistoryMessage{MessageID: id, Timestamp: quando}
}

func executar(t *testing.T, r *readerStub, res appport.LIDResolver, jid string) (*chat.ChatHistoryResult, error) {
	t.Helper()
	uc := chat.NewGetChatHistoryUseCase(r, &contractsfake.Logger{}).WithLIDResolver(res)
	return uc.Execute(context.Background(), lidUser, 50, chat.ChatHistoryQuery{ChatJID: jid})
}

// A leitura da chave alternativa falha: a resposta tem de continuar a ser a que
// existia antes da F183. Devolver o erro faria uma correção transformar um
// resultado parcial num 500 — pior do que o defeito que ela conserta.
func TestChatHistoryLID_LeituraAlternativaEmFalhaPreservaOPrimario(t *testing.T) {
	const lid, pn = "1@lid", "55@s.whatsapp.net"
	r := &readerStub{
		porChave: map[string][]appport.ChatHistoryMessage{lid: {msg("SOB-LID", time.Now())}},
		falhaEm:  map[string]error{pn: errors.New("banco indisponivel")},
	}

	got, err := executar(t, r, resolverStub{pn: pn}, lid)
	if err != nil {
		t.Fatalf("falha na chave alternativa derrubou a leitura: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].MessageID != "SOB-LID" {
		t.Fatalf("mensagens = %+v, quero so' a de sob o LID", got.Messages)
	}
}

// A chave alternativa existe mas está vazia: nada a fundir, e o primário passa
// intacto — sem reordenar nem truncar por engano.
func TestChatHistoryLID_AlternativaVaziaNaoMexeNoPrimario(t *testing.T) {
	const lid, pn = "1@lid", "55@s.whatsapp.net"
	agora := time.Now()
	r := &readerStub{porChave: map[string][]appport.ChatHistoryMessage{
		lid: {msg("A", agora), msg("B", agora.Add(-time.Hour))},
		pn:  {},
	}}

	got, err := executar(t, r, resolverStub{pn: pn}, lid)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got.Messages) != 2 || got.Messages[0].MessageID != "A" || got.Messages[1].MessageID != "B" {
		t.Fatalf("mensagens = %+v, quero [A B] na ordem original", got.Messages)
	}
}

// O resolvedor devolve o MESMO identificador: não pode haver segunda leitura.
// Sem esta guarda, a mesma chave seria lida duas vezes e cada mensagem entraria
// na deduplicação — trabalho dobrado para produzir exatamente o mesmo resultado,
// e um caminho a mais para o banco em toda leitura de chat @lid.
func TestChatHistoryLID_ResolucaoIdenticaNaoRelê(t *testing.T) {
	const lid = "1@lid"
	r := &readerStub{porChave: map[string][]appport.ChatHistoryMessage{lid: {msg("A", time.Now())}}}

	if _, err := executar(t, r, resolverStub{pn: lid}, lid); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(r.lidas) != 1 {
		t.Fatalf("leituras = %v, quero exatamente 1: resolver para a propria chave nao pode reler", r.lidas)
	}
}

// A mesma mensagem sob as DUAS chaves aparece UMA vez. O índice único é
// (user_id, message_id) e não impede a mesma conversa de estar arquivada sob
// dois chat_jid — tratá-lo como se impedisse produziria duplicados na resposta.
func TestChatHistoryLID_MensagemNasDuasChavesNaoDuplica(t *testing.T) {
	const lid, pn = "1@lid", "55@s.whatsapp.net"
	agora := time.Now()
	r := &readerStub{porChave: map[string][]appport.ChatHistoryMessage{
		lid: {msg("MESMA", agora)},
		pn:  {msg("MESMA", agora), msg("OUTRA", agora.Add(-time.Hour))},
	}}

	got, err := executar(t, r, resolverStub{pn: pn}, lid)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("mensagens = %d, quero 2: a mensagem nas duas chaves foi duplicada", len(got.Messages))
	}
}
