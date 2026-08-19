package message_test

import (
	"context"
	"errors"
	"testing"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// errSession é o erro que a porta devolve quando não há sessão wa-noise. O
// que os testes cobram dos use cases é que ele chegue INTEIRO ao chamador —
// identidade preservada por errors.Is, e não um texto novo que apague a
// causa (era o que fmt.Errorf("no session") fazia).
var errSession = errors.New("porta: sessao inexistente")

// errGenID é a falha da geração de ID. Vem da mesma porta, mas de outro
// método, e por isso precisa de identidade própria: o teste que a usa
// verifica justamente que o use case não confunde os dois caminhos.
var errGenID = errors.New("porta: gerador de id indisponivel")

const (
	callerID = "id-vindo-do-chamador"
	txtID    = "user-1"
)

// composerUC descreve um dos use cases send_* que AINDA são construídos
// sobre port.MessageComposer. Todos têm a mesma forma — validar campos,
// exigir sessão, gerar ID quando o chamador não deu um — e por isso são
// exercidos por uma tabela só, em vez de um bloco de teste por use case.
type composerUC struct {
	name string
	// infoMsg é a mensagem do log de sucesso.
	infoMsg string
	// run executa o use case com um request VÁLIDO cujo campo Id vale id.
	run func(mc port.MessageComposer, l port.Logger, id string) (msgID, status string, err error)
	// missing traz um request inválido por campo obrigatório.
	missing []missingField
}

// missingField é um request a que falta exatamente um campo obrigatório.
type missingField struct {
	field string
	run   func(mc port.MessageComposer, l port.Logger) error
}

// composerUseCases cobria SendContact e SendLocation até CAP-08A/CAP-08B,
// SendPoll até o CAP-14 e SendTemplate até o CAP-15, migrá-los para
// port.SimpleMessenger (envio de verdade, não mais "validated"). Os eixos que
// este arquivo cobria para eles — validação de campo obrigatório, propagação
// de falha de sessão, geração de ID no caminho feliz, respeito ao Id do
// chamador — foram migrados para send_location_test.go, send_contact_test.go,
// send_poll_test.go e send_template_test.go, com a mesma disciplina de causa
// (estrutura entregue à porta) que os demais use cases send_* já exigem.
// Nenhum eixo foi removido, só realocado com o tipo de porta.
//
// O destino de cada eixo do SendTemplate, nome por nome:
//
//	campo obrigatório ausente   TestSendTemplate_MissingRequiredField
//	falha de sessão             TestSendTemplate_SessionFailurePropagates
//	Id do chamador respeitado   TestSendTemplate_MessageIDIsTheOneActuallySent
//	caminho feliz               TestSendTemplate_CausalSuccess
//
// A "geração de ID no caminho feliz" não tem destino porque deixou de
// existir: o use case não gera mais ID nenhum: o MessageID publicado é o que
// a porta devolveu (o que o SDK REALMENTE usou), como nas outras oito
// capabilities já migradas.
//
// SendButtons SAIU no CAP-21, pelo mesmo motivo: passou a consumir
// port.InteractiveMessenger + port.JIDResolver + port.MediaFetcher e a
// enviar de verdade. O destino de cada eixo, nome por nome, em
// send_buttons_test.go:
//
//	campo obrigatório ausente   TestSendButtons_MissingRequiredField
//	falha de sessão             TestSendButtons_SessionFailurePropagates
//	Id do chamador respeitado   TestSendButtons_MessageIDIsTheOneActuallySent
//	caminho feliz               TestSendButtons_CausalSuccess
//
// Os DOIS eixos de geração de ID (TestComposerUseCases_SuccessGeneratesID e
// TestComposerUseCases_MessageIDFailurePropagates) não têm destino porque
// deixaram de existir para esta rota, exatamente como aconteceu com
// SendTemplate: o use case não chama mais NewMessageID, e o MessageID
// publicado é o que a porta devolveu.
//
// A tabela permanece com SendList, o ÚNICO caso que sobrou. Deixá-la com
// zero entradas seria pior que removê-la: cada `for` deste arquivo passaria
// a iterar sobre nada e a suíte inteira ficaria verde sem medir coisa
// alguma.
func composerUseCases() []composerUC {
	return []composerUC{
		{
			name:    "SendList",
			infoMsg: "list validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendListUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendListRequest{Phone: "5511987654321", Desc: "Itens", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendListUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendListRequest{Desc: "Itens"})
					return err
				}},
				{"Desc", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendListUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendListRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
	}
}

// resultOf desembrulha o par (result, err) de um use case send_* sem
// dereferenciar um ponteiro nil no caminho de erro. read é chamada apenas
// quando err é nil.
func resultOf(err error, read func() (string, string)) (string, string, error) {
	if err != nil {
		return "", "", err
	}
	msgID, status := read()
	return msgID, status, nil
}

// TestComposerUseCases_MissingRequiredField: campo obrigatório ausente é
// recusado ANTES de a porta ser tocada. O efeito observável (EnsureSession
// nunca chamado) é o que distingue este caminho do de sessão — não o texto.
func TestComposerUseCases_MissingRequiredField(t *testing.T) {
	for _, uc := range composerUseCases() {
		for _, mf := range uc.missing {
			t.Run(uc.name+"/"+mf.field, func(t *testing.T) {
				mc := &contractsfake.MessageComposer{}
				logger := &contractsfake.Logger{}

				err := mf.run(mc, logger)

				if err == nil {
					t.Fatalf("request sem %s foi aceito", mf.field)
				}
				if n := len(mc.EnsureSessionCalls); n != 0 {
					t.Errorf("validacao falhou mas a porta foi consultada %d vez(es)", n)
				}
				if n := len(mc.NewMessageIDCalls); n != 0 {
					t.Errorf("validacao falhou mas um ID foi gerado (%d chamada(s))", n)
				}
				if n := logger.Len(); n != 0 {
					t.Errorf("erro de validacao gerou %d registro(s) de log: %v", n, logger.Messages())
				}
			})
		}
	}
}

// TestComposerUseCases_SessionFailurePropagates trava a migração dos sites
// fmt.Errorf("no session"): o erro da porta chega ao chamador por
// identidade, e o log de saída carrega a causa.
func TestComposerUseCases_SessionFailurePropagates(t *testing.T) {
	for _, uc := range composerUseCases() {
		t.Run(uc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{SessionGuard: contractsfake.FailSession(errSession)}
			logger := &contractsfake.Logger{}

			_, _, err := uc.run(mc, logger, "")

			if !errors.Is(err, errSession) {
				t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
			}
			if n := len(mc.NewMessageIDCalls); n != 0 {
				t.Errorf("sem sessao, mas um ID foi gerado (%d chamada(s))", n)
			}
			assertSessionLog(t, logger, "txtID", txtID)
		})
	}
}

// TestComposerUseCases_MessageIDFailurePropagates cobre o segundo caminho de
// saída: a sessão existe, mas a geração de ID falha. Antes da migração ele
// devolvia o mesmo "no session" do caminho anterior — dois modos de falha
// distintos indistinguíveis pelo chamador.
func TestComposerUseCases_MessageIDFailurePropagates(t *testing.T) {
	for _, uc := range composerUseCases() {
		t.Run(uc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{
				NewMessageIDFunc: func(context.Context, string) (string, error) { return "", errGenID },
			}
			logger := &contractsfake.Logger{}

			_, _, err := uc.run(mc, logger, "")

			if !errors.Is(err, errGenID) {
				t.Fatalf("falha de geracao de ID nao chegou ao chamador: got %#v", err)
			}
			if errors.Is(err, errSession) {
				t.Fatal("falha de geracao de ID foi confundida com falha de sessao")
			}
			rec := requireLog(t, logger, contractsfake.LevelError, "failed to generate message ID")
			if got, _ := rec.Keyval("error"); got != error(errGenID) {
				t.Errorf("log nao carrega a causa: %v", rec.Keyvals)
			}
		})
	}
}

// TestComposerUseCases_SuccessGeneratesID: sem Id no request, o use case
// pede um à porta e devolve exatamente o que ela deu.
func TestComposerUseCases_SuccessGeneratesID(t *testing.T) {
	for _, uc := range composerUseCases() {
		t.Run(uc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			logger := &contractsfake.Logger{}

			msgID, status, err := uc.run(mc, logger, "")

			if err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if msgID != contractsfake.DefaultMessageID {
				t.Errorf("MessageID: got %q, want %q", msgID, contractsfake.DefaultMessageID)
			}
			if status != "validated" {
				t.Errorf("Status: got %q, want %q", status, "validated")
			}
			if n := len(mc.NewMessageIDCalls); n != 1 {
				t.Fatalf("NewMessageID chamado %d vez(es), esperava 1", n)
			}
			if got := mc.NewMessageIDCalls[0].TxtID; got != txtID {
				t.Errorf("NewMessageID recebeu txtID %q, esperava %q", got, txtID)
			}
			rec := requireLog(t, logger, contractsfake.LevelInfo, uc.infoMsg)
			if got, ok := rec.Keyval("msgID"); !ok || got != contractsfake.DefaultMessageID {
				t.Errorf("log de sucesso nao carrega o msgID gerado: %v", rec.Keyvals)
			}
		})
	}
}

// TestComposerUseCases_SuccessKeepsCallerID: com Id no request, o use case o
// respeita e NÃO gera outro — a idempotência que o chamador comprou ao
// mandar o seu.
func TestComposerUseCases_SuccessKeepsCallerID(t *testing.T) {
	for _, uc := range composerUseCases() {
		t.Run(uc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			logger := &contractsfake.Logger{}

			msgID, status, err := uc.run(mc, logger, callerID)

			if err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if msgID != callerID {
				t.Errorf("MessageID: got %q, want %q", msgID, callerID)
			}
			if status != "validated" {
				t.Errorf("Status: got %q, want %q", status, "validated")
			}
			if n := len(mc.NewMessageIDCalls); n != 0 {
				t.Errorf("Id do chamador foi ignorado: NewMessageID chamado %d vez(es)", n)
			}
		})
	}
}

// --- helpers de log ----------------------------------------------------

// requireLog exige um registro com nível e mensagem dados, e o devolve.
func requireLog(t *testing.T, logger *contractsfake.Logger, level, msg string) contractsfake.LogRecord {
	t.Helper()
	rec, ok := logger.FindLevel(level, msg)
	if !ok {
		t.Fatalf("faltou log %s %q; houve: %v", level, msg, logger.Messages())
	}
	if !rec.IsStructured() {
		t.Errorf("log %q nao e' estruturado (keyvals impares ou vazios): %v", msg, rec.Keyvals)
	}
	return rec
}

// assertSessionLog cobra a forma do log de ausência de sessão: nível error,
// mensagem canônica, a causa real e o identificador da sessão.
func assertSessionLog(t *testing.T, logger *contractsfake.Logger, idKey, idValue string) {
	t.Helper()
	rec := requireLog(t, logger, contractsfake.LevelWarn, "no wanoise session")
	if got, ok := rec.Keyval("error"); !ok || got != error(errSession) {
		t.Errorf("log de sessao nao carrega a causa real: %v", rec.Keyvals)
	}
	if got, ok := rec.Keyval(idKey); !ok || got != idValue {
		t.Errorf("log de sessao nao carrega %s=%q: %v", idKey, idValue, rec.Keyvals)
	}
}
