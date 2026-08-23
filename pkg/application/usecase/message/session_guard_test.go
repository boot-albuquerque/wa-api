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

// guardUC descreve os 2 use cases de MUTAÇÃO de mensagem já existente
// (delete, edit). Não geram ID — devolvem o ID da mensagem ALVO, que o
// request trouxe — e por isso não cabem na tabela de composerUseCases.
//
// Em CAP-10 os dois deixaram de ter port.SessionGuard como única porta:
// passaram a depender de port.ChatMessenger (que embute SessionGuard) e de
// port.JIDResolver, porque deixaram de devolver "validated" e passaram a
// revogar/editar de verdade. Os três eixos que a tabela cobria —
// campo obrigatório ausente sem consultar a porta, falha de sessão
// propagada com log, e caminho feliz com EnsureSession exatamente uma vez —
// continuam AQUI, agora sobre a porta de verdade. Os eixos NOVOS que a
// mutação trouxe (falha do envio, JID que não parseia, argumentos exatos
// entregues à porta) ficam em delete_edit_message_test.go.
//
// Os 5 download_* SAÍRAM desta tabela em CAP-09B: eles deixaram de ter
// port.SessionGuard como única porta (agora dependem de
// port.MediaDownloader, que embute SessionGuard) e deixaram de devolver um
// resultado vazio. Os três eixos que cobriam aqui — campo obrigatório
// ausente sem consultar a porta, falha de sessão propagada com log, e
// caminho feliz com EnsureSession exatamente uma vez — foram realocados,
// nome por nome para as cinco capabilities, em download_media_test.go.
type guardUC struct {
	name string
	// infoMsg é a mensagem do log de sucesso.
	infoMsg string
	// wantMsgID é o MessageID esperado no caminho feliz.
	wantMsgID string
	// run executa o use case com um request VÁLIDO e devolve o MessageID do
	// resultado.
	run func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) (string, error)
	// missing traz um request inválido por campo obrigatório.
	missing []guardMissingField
}

type guardMissingField struct {
	field string
	run   func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) error
	// afterPort marca o campo cuja guarda roda DEPOIS da porta, e não antes.
	// Só o Id de SendEditMessage é assim, e não por descuido: o histórico
	// (`git show 41bc8e2^:handlers.go`, linhas 2919, 2924, 2929 e 2936)
	// valida Phone, Body, o PARSE do Phone e só então o Id — logo a resolução
	// do JID (e, na nossa forma, a sessão que a precede) acontece antes de o
	// Id ausente ser recusado. A ordem é observável pela causa devolvida e
	// está travada em
	// handlers.TestSendEditMessage_PhoneParseRejectedBeforeMissingID.
	afterPort bool
}

const editedID = "3EB0ABC123"

func guardUseCases() []guardUC {
	out := make([]guardUC, 0, 2)

	out = append(out,
		guardUC{
			name:      "DeleteMessage",
			infoMsg:   "message deleted",
			wantMsgID: editedID,
			run: func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) (string, error) {
				r, err := message.NewDeleteMessageUseCase(cm, jr, l).Execute(context.Background(), txtID,
					domain.DeleteMessageRequest{Phone: "5511987654321", ID: editedID})
				if err != nil {
					return "", err
				}
				if r.Status != domain.StatusDeleted {
					return "", errors.New("status inesperado: " + r.Status)
				}
				return r.MessageID, nil
			},
			missing: []guardMissingField{
				{field: "Phone", run: func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) error {
					_, err := message.NewDeleteMessageUseCase(cm, jr, l).Execute(context.Background(), txtID,
						domain.DeleteMessageRequest{ID: editedID})
					return err
				}},
				{field: "Id", run: func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) error {
					_, err := message.NewDeleteMessageUseCase(cm, jr, l).Execute(context.Background(), txtID,
						domain.DeleteMessageRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
		guardUC{
			name:      "SendEditMessage",
			infoMsg:   "message edit sent",
			wantMsgID: editedID,
			run: func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) (string, error) {
				r, err := message.NewSendEditMessageUseCase(cm, jr, l).Execute(context.Background(), txtID,
					domain.SendEditMessageRequest{Phone: "5511987654321", Body: "Corrigido", ID: editedID})
				if err != nil {
					return "", err
				}
				if r.Status != domain.StatusSent {
					return "", errors.New("status inesperado: " + r.Status)
				}
				return r.MessageID, nil
			},
			missing: []guardMissingField{
				{field: "Phone", run: func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) error {
					_, err := message.NewSendEditMessageUseCase(cm, jr, l).Execute(context.Background(), txtID,
						domain.SendEditMessageRequest{Body: "Corrigido", ID: editedID})
					return err
				}},
				{field: "Body", run: func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) error {
					_, err := message.NewSendEditMessageUseCase(cm, jr, l).Execute(context.Background(), txtID,
						domain.SendEditMessageRequest{Phone: "5511987654321", ID: editedID})
					return err
				}},
				{field: "Id", run: func(cm *contractsfake.ChatMessenger, jr port.JIDResolver, l port.Logger) error {
					_, err := message.NewSendEditMessageUseCase(cm, jr, l).Execute(context.Background(), txtID,
						domain.SendEditMessageRequest{Phone: "5511987654321", Body: "Corrigido"})
					return err
				}, afterPort: true},
			},
		},
	)
	return out
}

// TestGuardUseCases_MissingRequiredField: nestes use cases a validação
// precede a porta — exceto o Id de SendEditMessage, cuja guarda o histórico
// põe DEPOIS do parse do Phone (ver guardMissingField.afterPort). É o número
// de chamadas a EnsureSession que separa os dois casos; em nenhum deles a
// MUTAÇÃO pode acontecer.
func TestGuardUseCases_MissingRequiredField(t *testing.T) {
	for _, uc := range guardUseCases() {
		for _, mf := range uc.missing {
			t.Run(uc.name+"/"+mf.field, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{}
				jr := &contractsfake.JIDResolver{}
				logger := &contractsfake.Logger{}

				err := mf.run(cm, jr, logger)

				if err == nil {
					t.Fatalf("request sem %s foi aceito", mf.field)
				}
				wantSessionCalls := 0
				if mf.afterPort {
					wantSessionCalls = 1
				}
				if n := len(cm.EnsureSessionCalls); n != wantSessionCalls {
					t.Errorf("validacao de %s: EnsureSession chamado %d vez(es), quero %d "+
						"(afterPort=%v)", mf.field, n, wantSessionCalls, mf.afterPort)
				}
				if n := len(cm.RevokeMessageCalls) + len(cm.EditMessageCalls); n != 0 {
					t.Errorf("validacao falhou mas a mutacao aconteceu %d vez(es)", n)
				}
				if n := logger.Len(); n != 0 {
					t.Errorf("erro de validacao gerou %d registro(s) de log: %v", n, logger.Messages())
				}
			})
		}
	}
}

// TestGuardUseCases_SessionFailurePropagates: mesma exigência da tabela de
// composer, sobre a outra porta.
func TestGuardUseCases_SessionFailurePropagates(t *testing.T) {
	for _, uc := range guardUseCases() {
		t.Run(uc.name, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errSession)}
			jr := &contractsfake.JIDResolver{}
			logger := &contractsfake.Logger{}

			_, err := uc.run(cm, jr, logger)

			if !errors.Is(err, errSession) {
				t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
			}
			assertSessionLog(t, logger, "txtID", txtID)
			if n := len(cm.RevokeMessageCalls) + len(cm.EditMessageCalls); n != 0 {
				t.Errorf("sessao invalida mas a mutacao aconteceu %d vez(es)", n)
			}
		})
	}
}

// TestGuardUseCases_Success: caminho feliz — a sessão é consultada com o
// txtID recebido, a mutação acontece, o resultado sai com o Status certo e
// o log de sucesso acontece.
func TestGuardUseCases_Success(t *testing.T) {
	for _, uc := range guardUseCases() {
		t.Run(uc.name, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{}
			jr := &contractsfake.JIDResolver{}
			logger := &contractsfake.Logger{}

			msgID, err := uc.run(cm, jr, logger)

			if err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if msgID != uc.wantMsgID {
				t.Errorf("MessageID: got %q, want %q", msgID, uc.wantMsgID)
			}
			if n := len(cm.EnsureSessionCalls); n != 1 {
				t.Fatalf("EnsureSession chamado %d vez(es), esperava 1", n)
			}
			if got := cm.EnsureSessionCalls[0].TxtID; got != txtID {
				t.Errorf("EnsureSession recebeu txtID %q, esperava %q", got, txtID)
			}
			requireLog(t, logger, contractsfake.LevelInfo, uc.infoMsg)
		})
	}
}
