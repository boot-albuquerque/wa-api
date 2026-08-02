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

// errSession é o erro que a porta devolve quando não há sessão whatsmeow. O
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

// composerUC descreve um dos 12 use cases send_* construídos sobre
// port.MessageComposer. Todos têm a mesma forma — validar campos, exigir
// sessão, gerar ID quando o chamador não deu um — e por isso são exercidos
// por uma tabela só, em vez de 12 blocos de teste quase idênticos.
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

func composerUseCases() []composerUC {
	return []composerUC{
		{
			name:    "SendMessage",
			infoMsg: "message validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendMessageUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendMessageRequest{Phone: "5511987654321", Body: "Ola", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendMessageUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendMessageRequest{Body: "Ola"})
					return err
				}},
				{"Body", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendMessageUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendMessageRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
		{
			name:    "SendImage",
			infoMsg: "image validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendImageUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendImageRequest{Phone: "5511987654321", Image: "data:image/png;base64,abc", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendImageUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendImageRequest{Image: "data:image/png;base64,abc"})
					return err
				}},
				{"Image", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendImageUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendImageRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
		{
			name:    "SendDocument",
			infoMsg: "document validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendDocumentUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendDocumentRequest{Phone: "5511987654321", Document: "data:...", FileName: "a.pdf", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendDocumentUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendDocumentRequest{Document: "data:...", FileName: "a.pdf"})
					return err
				}},
				{"Document", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendDocumentUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendDocumentRequest{Phone: "5511987654321", FileName: "a.pdf"})
					return err
				}},
				{"FileName", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendDocumentUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendDocumentRequest{Phone: "5511987654321", Document: "data:..."})
					return err
				}},
			},
		},
		{
			name:    "SendAudio",
			infoMsg: "audio validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendAudioUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendAudioRequest{Phone: "5511987654321", Audio: "data:...", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendAudioUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendAudioRequest{Audio: "data:..."})
					return err
				}},
				{"Audio", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendAudioUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendAudioRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
		{
			name:    "SendSticker",
			infoMsg: "sticker validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendStickerUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendStickerRequest{Phone: "5511987654321", Sticker: "data:...", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendStickerUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendStickerRequest{Sticker: "data:..."})
					return err
				}},
				{"Sticker", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendStickerUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendStickerRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
		{
			name:    "SendVideo",
			infoMsg: "video validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendVideoUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendVideoRequest{Phone: "5511987654321", Video: "data:...", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendVideoUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendVideoRequest{Video: "data:..."})
					return err
				}},
				{"Video", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendVideoUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendVideoRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
		{
			name:    "SendContact",
			infoMsg: "contact validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendContactUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendContactRequest{Phone: "5511987654321", Name: "Ana", Vcard: "BEGIN:VCARD", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendContactUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendContactRequest{Name: "Ana", Vcard: "BEGIN:VCARD"})
					return err
				}},
				{"Name", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendContactUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendContactRequest{Phone: "5511987654321", Vcard: "BEGIN:VCARD"})
					return err
				}},
				{"Vcard", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendContactUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendContactRequest{Phone: "5511987654321", Name: "Ana"})
					return err
				}},
			},
		},
		{
			name:    "SendLocation",
			infoMsg: "location validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendLocationUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendLocationRequest{Phone: "5511987654321", Latitude: -23.5, Longitude: -46.6, ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendLocationUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendLocationRequest{Latitude: -23.5, Longitude: -46.6})
					return err
				}},
				{"Latitude", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendLocationUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendLocationRequest{Phone: "5511987654321", Longitude: -46.6})
					return err
				}},
				{"Longitude", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendLocationUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendLocationRequest{Phone: "5511987654321", Latitude: -23.5})
					return err
				}},
			},
		},
		{
			name:    "SendButtons",
			infoMsg: "buttons validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendButtonsUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendButtonsRequest{Phone: "5511987654321", Body: "Escolha", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendButtonsUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendButtonsRequest{Body: "Escolha"})
					return err
				}},
				{"Body", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendButtonsUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendButtonsRequest{Phone: "5511987654321"})
					return err
				}},
			},
		},
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
		{
			name:    "SendPoll",
			infoMsg: "poll validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendPollUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendPollRequest{Group: "grupo@g.us", Header: "Qual?", Options: []string{"a", "b"}, ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Group", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendPollUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendPollRequest{Header: "Qual?", Options: []string{"a", "b"}})
					return err
				}},
				{"Header", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendPollUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendPollRequest{Group: "grupo@g.us", Options: []string{"a", "b"}})
					return err
				}},
				// Options é o único campo cuja regra não é "não vazio": uma
				// enquete de uma opção só é tão inválida quanto nenhuma.
				{"Options com apenas uma", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendPollUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendPollRequest{Group: "grupo@g.us", Header: "Qual?", Options: []string{"a"}})
					return err
				}},
			},
		},
		{
			name:    "SendTemplate",
			infoMsg: "template validated",
			run: func(mc port.MessageComposer, l port.Logger, id string) (string, string, error) {
				r, err := message.NewSendTemplateUseCase(mc, l).Execute(context.Background(), txtID,
					domain.SendTemplateRequest{Phone: "5511987654321", Content: "Corpo", Footer: "Rodape", ID: id})
				return resultOf(err, func() (string, string) { return r.MessageID, r.Status })
			},
			missing: []missingField{
				{"Phone", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendTemplateUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendTemplateRequest{Content: "Corpo", Footer: "Rodape"})
					return err
				}},
				{"Content", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendTemplateUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendTemplateRequest{Phone: "5511987654321", Footer: "Rodape"})
					return err
				}},
				{"Footer", func(mc port.MessageComposer, l port.Logger) error {
					_, err := message.NewSendTemplateUseCase(mc, l).Execute(context.Background(), txtID,
						domain.SendTemplateRequest{Phone: "5511987654321", Content: "Corpo"})
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
	rec := requireLog(t, logger, contractsfake.LevelError, "no whatsmeow session")
	if got, ok := rec.Keyval("error"); !ok || got != error(errSession) {
		t.Errorf("log de sessao nao carrega a causa real: %v", rec.Keyvals)
	}
	if got, ok := rec.Keyval(idKey); !ok || got != idValue {
		t.Errorf("log de sessao nao carrega %s=%q: %v", idKey, idValue, rec.Keyvals)
	}
}
