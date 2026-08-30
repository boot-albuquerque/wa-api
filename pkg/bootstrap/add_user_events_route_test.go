package bootstrap

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"wa-api/pkg/domain"
)

// CAP-33 — POST /admin/users e a lista `events` (HOUSEKEEP F159).
//
// A recusa de evento desconhecido é contrato NOVO, decidido no canal: até
// esta sessão `isValidEvent` devolvia true sempre
// (pkg/application/usecase/user/add_user.go:179), e o 400 logo abaixo dela
// era letra morta. Quem manda "Mesage" recebia 200.
//
// Os testes vão pela ROTA REGISTRADA (registerAdminRoutes + gorilla/mux)
// contra o SQLite real, e não pelo use case, porque o eixo aqui é de
// fronteira: o status 400 e a mensagem do envelope são derivados do
// apperr.Category em pkg/presentation/http/response.go:53, não escritos pelo
// handler — um teste de use case não veria nem o status nem o corpo.
//
// O fixture (newAddUserRouteFixture) e a asserção de "não foi criado" vêm de
// add_user_hmac_route_test.go: a busca é por token_hash porque a coluna
// `token` recebe VAZIO desde a F97.

// addUserEventsToken é o token usado pelos casos de evento; distinto do
// addUserTestToken da CAP-28 para que os dois arquivos não colidam.
const addUserEventsToken = "token-cap33"

// storedEvents devolve a coluna users.events do usuário do token dado, e se
// a linha existe.
func (f *addUserRouteFixture) storedEvents(t *testing.T, token string) (string, bool) {
	t.Helper()
	var rows []string
	if err := f.db.Select(&rows, f.db.Rebind(`SELECT events FROM users WHERE token_hash = ?`), domain.HashToken(token)); err != nil {
		t.Fatalf("select events: %v", err)
	}
	if len(rows) == 0 {
		return "", false
	}
	if len(rows) > 1 {
		t.Fatalf("linhas para o token %q = %d, queria no máximo 1", token, len(rows))
	}
	return rows[0], true
}

// addUserErrorEnvelope extrai {"error":{"code","message"}} do corpo.
func addUserErrorEnvelope(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("resposta não é JSON: %v (corpo: %s)", err, body)
	}
	if envelope.Success {
		t.Errorf("success = true, queria false (corpo: %s)", body)
	}
	return envelope.Error.Code, envelope.Error.Message
}

func TestAdminAddUser_EventosValidosCriamUsuario(t *testing.T) {
	tests := []struct {
		name   string
		events string // exatamente o que vai no corpo JSON
		want   string // o que tem de ficar gravado em users.events
	}{
		// O caminho de SUCESSO, e não só a recusa: se a guarda passar a
		// recusar demais, são estes que caem.
		{name: "um evento válido", events: `"Message"`, want: "Message"},
		{name: "vários eventos válidos", events: `"Message,ReadReceipt,Presence"`, want: "Message,ReadReceipt,Presence"},
		// "All" está na lista dos 48 (pkg/domain/constants.go:75) e é o valor
		// mais usado em produção — recusá-lo quebraria todo mundo.
		{name: "All", events: `"All"`, want: "All"},
		// Espaço em volta é aceito porque o laço faz TrimSpace antes de
		// validar (add_user.go:88). O valor GRAVADO continua sendo o
		// original, com os espaços: a validação não reescreve o campo, e
		// mudar isso seria alterar um segundo contrato de carona.
		{name: "espaços em volta", events: `"  Message  "`, want: "  Message  "},
		// Lista vazia é o caminho que NÃO pode quebrar: o bloco inteiro de
		// validação fica de fora quando req.Events == "".
		{name: "events vazio", events: `""`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAddUserRouteFixture(t)
			rec := f.postUser(t, `{"name":"alice","token":"`+addUserEventsToken+`","events":`+tt.events+`,"engine":"noise"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, queria %d (corpo: %s)", rec.Code, http.StatusOK, rec.Body.String())
			}
			stored, ok := f.storedEvents(t, addUserEventsToken)
			if !ok {
				t.Fatal("usuário não foi criado")
			}
			if stored != tt.want {
				t.Errorf("users.events = %q, queria %q", stored, tt.want)
			}
		})
	}
}

func TestAdminAddUser_EventoDesconhecidoERecusadoENadaEGravado(t *testing.T) {
	tests := []struct {
		name        string
		events      string
		wantInvalid string // o evento que tem de aparecer na mensagem
	}{
		{
			name:        "evento com typo",
			events:      `"Mesage"`,
			wantInvalid: "Mesage",
		},
		{
			// O eixo que separa RECUSAR de FILTRAR. Um filtro silencioso
			// gravaria "Message" e devolveria 200; a recusa não grava nada.
			// Se alguém trocar a decisão por filtro, é este que cai.
			name:        "lista mista, um válido e um inválido",
			events:      `"Message,Mesage"`,
			wantInvalid: "Mesage",
		},
		{
			// O inválido vindo PRIMEIRO: prova que a recusa não depende de
			// já ter aceitado alguma coisa antes.
			name:        "lista mista, o inválido primeiro",
			events:      `"Mesage,Message"`,
			wantInvalid: "Mesage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAddUserRouteFixture(t)
			rec := f.postUser(t, `{"name":"alice","token":"`+addUserEventsToken+`","events":`+tt.events+`,"engine":"noise"}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, queria %d (corpo: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			code, message := addUserErrorEnvelope(t, rec.Body.Bytes())
			if code != "invalid_event_type" {
				t.Errorf("error.code = %q, queria %q", code, "invalid_event_type")
			}
			if want := "invalid event type: " + tt.wantInvalid; message != want {
				t.Errorf("error.message = %q, queria %q", message, want)
			}
			// Falha FECHADA, e a ORDEM é o contrato: validar ANTES de
			// gravar. Se a validação corresse depois do CreateUser, o
			// usuário nasceria e o cliente veria 400 — o pior dos dois.
			if stored, ok := f.storedEvents(t, addUserEventsToken); ok {
				t.Errorf("usuário criado com evento inválido (users.events = %q)", stored)
			}
			// E a lista mista não pode ter virado uma gravação parcial em
			// nenhuma linha: nenhuma linha de usuário existe.
			var total int
			if err := f.db.Get(&total, `SELECT COUNT(*) FROM users`); err != nil {
				t.Fatalf("count users: %v", err)
			}
			if total != 0 {
				t.Errorf("linhas em users = %d, queria 0", total)
			}
			// A mensagem não vaza o token do corpo.
			if strings.Contains(rec.Body.String(), addUserEventsToken) {
				t.Errorf("o token apareceu na resposta de erro: %s", rec.Body.String())
			}
		})
	}
}
