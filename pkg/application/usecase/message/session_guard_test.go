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

// guardUC descreve os use cases desta pasta cuja única porta é
// port.SessionGuard: os 5 download_* e os 2 de manipulação de mensagem já
// existente (delete, edit). Não geram ID — devolvem o que o request trouxe —
// e por isso não cabem na tabela de composerUseCases.
type guardUC struct {
	name string
	// infoMsg é a mensagem do log de sucesso.
	infoMsg string
	// wantMsgID é o MessageID esperado no caminho feliz. Vazio para os
	// download_*, cujo resultado não tem esse campo.
	wantMsgID string
	// run executa o use case com um request VÁLIDO e devolve o MessageID do
	// resultado (vazio quando o resultado não tem um).
	run func(sg port.SessionGuard, l port.Logger) (string, error)
	// missing traz um request inválido por campo obrigatório.
	missing []guardMissingField
}

type guardMissingField struct {
	field string
	run   func(sg port.SessionGuard, l port.Logger) error
}

const editedID = "3EB0ABC123"

func guardUseCases() []guardUC {
	// Os 5 download_* são byte a byte a mesma função, com outra mensagem de
	// log. A tabela reflete isso em vez de repetir o literal 5 vezes.
	downloads := []struct {
		name    string
		infoMsg string
		exec    func(sg port.SessionGuard, l port.Logger, req domain.DownloadRequest) (*domain.DownloadResult, error)
	}{
		{"DownloadImage", "download image validated", func(sg port.SessionGuard, l port.Logger, req domain.DownloadRequest) (*domain.DownloadResult, error) {
			return message.NewDownloadImageUseCase(sg, l).Execute(context.Background(), txtID, req)
		}},
		{"DownloadVideo", "download video validated", func(sg port.SessionGuard, l port.Logger, req domain.DownloadRequest) (*domain.DownloadResult, error) {
			return message.NewDownloadVideoUseCase(sg, l).Execute(context.Background(), txtID, req)
		}},
		{"DownloadAudio", "download audio validated", func(sg port.SessionGuard, l port.Logger, req domain.DownloadRequest) (*domain.DownloadResult, error) {
			return message.NewDownloadAudioUseCase(sg, l).Execute(context.Background(), txtID, req)
		}},
		{"DownloadDocument", "download document validated", func(sg port.SessionGuard, l port.Logger, req domain.DownloadRequest) (*domain.DownloadResult, error) {
			return message.NewDownloadDocumentUseCase(sg, l).Execute(context.Background(), txtID, req)
		}},
		{"DownloadSticker", "download sticker validated", func(sg port.SessionGuard, l port.Logger, req domain.DownloadRequest) (*domain.DownloadResult, error) {
			return message.NewDownloadStickerUseCase(sg, l).Execute(context.Background(), txtID, req)
		}},
	}

	out := make([]guardUC, 0, len(downloads)+2)
	for _, d := range downloads {
		exec := d.exec
		out = append(out, guardUC{
			name:    d.name,
			infoMsg: d.infoMsg,
			run: func(sg port.SessionGuard, l port.Logger) (string, error) {
				_, err := exec(sg, l, domain.DownloadRequest{URL: "https://mmg.whatsapp.net/x"})
				return "", err
			},
			missing: []guardMissingField{
				{"Url", func(sg port.SessionGuard, l port.Logger) error {
					_, err := exec(sg, l, domain.DownloadRequest{})
					return err
				}},
			},
		})
	}

	out = append(out,
		guardUC{
			name:      "DeleteMessage",
			infoMsg:   "message delete validated",
			wantMsgID: editedID,
			run: func(sg port.SessionGuard, l port.Logger) (string, error) {
				r, err := message.NewDeleteMessageUseCase(sg, l).Execute(context.Background(), txtID,
					domain.DeleteMessageRequest{Phone: "5511987654321", ID: editedID})
				if err != nil {
					return "", err
				}
				if r.Status != "validated" {
					t := errors.New("status inesperado: " + r.Status)
					return "", t
				}
				return r.MessageID, nil
			},
			missing: []guardMissingField{
				{"Phone", func(sg port.SessionGuard, l port.Logger) error {
					_, err := message.NewDeleteMessageUseCase(sg, l).Execute(context.Background(), txtID,
						domain.DeleteMessageRequest{ID: editedID})
					return err
				}},
				{"Id", func(sg port.SessionGuard, l port.Logger) error {
					_, err := message.NewDeleteMessageUseCase(sg, l).Execute(context.Background(), txtID,
						domain.DeleteMessageRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
		guardUC{
			name:      "SendEditMessage",
			infoMsg:   "message edit validated",
			wantMsgID: editedID,
			run: func(sg port.SessionGuard, l port.Logger) (string, error) {
				r, err := message.NewSendEditMessageUseCase(sg, l).Execute(context.Background(), txtID,
					domain.SendEditMessageRequest{Phone: "5511987654321", Body: "Corrigido", ID: editedID})
				if err != nil {
					return "", err
				}
				if r.Status != "validated" {
					return "", errors.New("status inesperado: " + r.Status)
				}
				return r.MessageID, nil
			},
			missing: []guardMissingField{
				{"Phone", func(sg port.SessionGuard, l port.Logger) error {
					_, err := message.NewSendEditMessageUseCase(sg, l).Execute(context.Background(), txtID,
						domain.SendEditMessageRequest{Body: "Corrigido", ID: editedID})
					return err
				}},
				{"Body", func(sg port.SessionGuard, l port.Logger) error {
					_, err := message.NewSendEditMessageUseCase(sg, l).Execute(context.Background(), txtID,
						domain.SendEditMessageRequest{Phone: "5511987654321", ID: editedID})
					return err
				}},
				{"Id", func(sg port.SessionGuard, l port.Logger) error {
					_, err := message.NewSendEditMessageUseCase(sg, l).Execute(context.Background(), txtID,
						domain.SendEditMessageRequest{Phone: "5511987654321", Body: "Corrigido"})
					return err
				}},
			},
		},
	)
	return out
}

// TestGuardUseCases_MissingRequiredField: nestes use cases a validação
// também precede a porta, e é o número de chamadas a EnsureSession que prova
// isso.
func TestGuardUseCases_MissingRequiredField(t *testing.T) {
	for _, uc := range guardUseCases() {
		for _, mf := range uc.missing {
			t.Run(uc.name+"/"+mf.field, func(t *testing.T) {
				sg := &contractsfake.SessionGuard{}
				logger := &contractsfake.Logger{}

				err := mf.run(sg, logger)

				if err == nil {
					t.Fatalf("request sem %s foi aceito", mf.field)
				}
				if n := len(sg.EnsureSessionCalls); n != 0 {
					t.Errorf("validacao falhou mas a porta foi consultada %d vez(es)", n)
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
			sg := contractsfake.FailSession(errSession)
			logger := &contractsfake.Logger{}

			_, err := uc.run(&sg, logger)

			if !errors.Is(err, errSession) {
				t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
			}
			assertSessionLog(t, logger, "txtID", txtID)
		})
	}
}

// TestGuardUseCases_Success: caminho feliz — a sessão é consultada com o
// txtID recebido, o resultado sai validado e o log de sucesso acontece.
func TestGuardUseCases_Success(t *testing.T) {
	for _, uc := range guardUseCases() {
		t.Run(uc.name, func(t *testing.T) {
			sg := &contractsfake.SessionGuard{}
			logger := &contractsfake.Logger{}

			msgID, err := uc.run(sg, logger)

			if err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if msgID != uc.wantMsgID {
				t.Errorf("MessageID: got %q, want %q", msgID, uc.wantMsgID)
			}
			if n := len(sg.EnsureSessionCalls); n != 1 {
				t.Fatalf("EnsureSession chamado %d vez(es), esperava 1", n)
			}
			if got := sg.EnsureSessionCalls[0].TxtID; got != txtID {
				t.Errorf("EnsureSession recebeu txtID %q, esperava %q", got, txtID)
			}
			requireLog(t, logger, contractsfake.LevelInfo, uc.infoMsg)
		})
	}
}
