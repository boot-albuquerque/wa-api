package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	wasession "wa-api/pkg/infra/noise/runtime/session"
	"wa-api/pkg/pairing"
)

// Este arquivo cobre a FRONTEIRA compartilhada por praticamente todos os
// handlers do pacote: quem esta autenticado, quem tem sessao, e o que sai no
// envelope quando um dos dois falta. A logica e' identica em ~30 handlers
// (metade via sessionUser, metade inline), e a duplicacao e' exatamente o
// motivo de valer um teste tabelado: uma divergencia entre as duas formas
// aparece aqui.
//
// A porta e' uma espia. Toda assercao negativa checa tambem que o use case
// NAO foi alcancado — sem isso, um handler que respondesse 401 DEPOIS de ja
// ter falado com o WhatsApp passaria no teste.

// spyPort implementa, de uma vez, as portas de capacidade que os handlers
// abaixo consomem. Registra se foi tocada.
type spyPort struct {
	calls int
	err   error
}

func (s *spyPort) EnsureSession(context.Context, string) error {
	s.calls++
	return s.err
}

// SetStatusMessage entrou com a F198, pelo mesmo motivo que Disconnect e
// Logout entraram com a F79: o use case só validava a sessão e devolvia 200
// sem chamar nada. É a TERCEIRA vez que este padrão aparece neste repositório.
// historySpy embrulha o spyPort para o teste de fronteira do history sync: o
// use case precisa da porta nova (F198) e o spy só sabe validar a sessão.
type historySpy struct{ *spyPort }

func (h *historySpy) RequestHistorySync(context.Context, string, appport.HistoryAnchor, int) (string, error) {
	return "REQ-SPY", h.err
}

func (s *spyPort) SetStatusMessage(context.Context, string, string) error {
	return s.err
}

// Disconnect e Logout entraram com a F79: os use cases correspondentes
// passaram a consumir SessionController, porque antes só validavam a sessão
// e devolviam 200 sem encerrar nada.
func (s *spyPort) Disconnect(context.Context, string) error {
	s.calls++
	return s.err
}

func (s *spyPort) Logout(context.Context, string) error {
	s.calls++
	return s.err
}

// IsPaired e RequestPairingCode entraram com o CAP-26: PairPhoneUseCase passou
// a consumir port.PhonePairer, porque antes só validava a sessão e devolvia
// 200 com LinkingCode VAZIO (F152).
func (s *spyPort) IsPaired(context.Context, string) (bool, error) {
	s.calls++
	return false, s.err
}

func (s *spyPort) RequestPairingCode(context.Context, string, string) (string, error) {
	s.calls++
	return "SPY-CODE", s.err
}

// PairingQR, CheckOwnership e StartSession entraram com a F281: GetQR, Connect
// e PairPhone deixaram de receber um use case pronto e passam a resolver a
// porta por engine (pkg/pairing). O spy satisfaz as tres portas para que
// "a porta foi tocada?" continue a ser UMA contagem.
func (s *spyPort) PairingQR(context.Context, string) (string, error) {
	s.calls++
	return "2@spy", s.err
}

func (s *spyPort) CheckOwnership(context.Context, string) error {
	s.calls++
	return s.err
}

func (s *spyPort) StartSession(context.Context, string, string) { s.calls++ }

// boundaryPairingRegistry liga o spy como provider do noise, com uma sessao
// "user-1" gravada nesse engine. A matriz consultada e' a REAL
// (capabilityregistry.NewCapabilityRegistry): um duble permissivo abencoaria
// caminhos que nao existem — ARMADILHAS.md #1.
func boundaryPairingRegistry(s *spyPort) *pairing.Registry {
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(_ context.Context, id string) ([]domain.UserListEntry, error) {
			if id == "" {
				return nil, nil
			}
			return []domain.UserListEntry{{ID: id, Engine: domain.EngineNoise}}, nil
		},
	}
	return pairing.NewRegistry(users, capabilityregistry.NewCapabilityRegistry(),
		&pairing.Provider{Engine: domain.EngineNoise, QRReader: s, PhonePairer: s, Starter: s},
		&pairing.Provider{Engine: domain.EngineHeadless})
}

// Detach entrou com a F80: o logout pela API agora solta a sessao depois de
// desvincular. Nao conta como toque na porta — o boundary test mede se o
// handler chegou a AGIR, e Detach so' ocorre depois de Logout ja ter agido.
func (s *spyPort) Detach(string) {}

func (s *spyPort) NewMessageID(context.Context, string) (string, error) {
	s.calls++
	return "generated-id", s.err
}

func (s *spyPort) MarkRead(context.Context, string, []string, time.Time, domain.JID, domain.JID) error {
	s.calls++
	return s.err
}

func (s *spyPort) SendReaction(context.Context, string, domain.JID, domain.Reaction) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) RevokeMessage(context.Context, string, domain.JID, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) EditMessage(context.Context, string, domain.JID, string, string, *domain.EditContextInfo) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) SendPollVote(context.Context, string, domain.JID, domain.PollVotePayload, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) SendText(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, *domain.ForwardContext, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

// Os CINCO métodos de port.SimpleMessenger entraram com o CAP-15
// (Location/Contact/Poll/Template) e o CAP-22 (List). A tabela só exercita
// SendTemplate; os outros quatro existem porque a interface é uma só e o
// use case a exige inteira — e ficam prontos para quando
// /chat/send/location, /contact, /poll e /list entrarem nesta tabela.
//
// Todos contam em s.calls: o boundary test mede se o handler chegou a AGIR
// sobre o WhatsApp, e enviar é o ato máximo.
func (s *spyPort) SendLocation(context.Context, string, domain.JID, domain.LocationPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) SendContact(context.Context, string, domain.JID, domain.ContactPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) SendPoll(context.Context, string, domain.JID, domain.PollPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) SendTemplate(context.Context, string, domain.JID, domain.TemplatePayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

func (s *spyPort) SendList(context.Context, string, domain.JID, domain.ListPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
	s.calls++
	return domain.MessageSendResult{}, s.err
}

// FetchLinkPreview implementa port.LinkPreviewFetcher. Não conta como
// "toque" em s.calls: é uma porta de resolução de metadata, não de ação
// sobre a sessão — os testes de fronteira medem se o handler AGIU no
// wa-noise, e uma consulta de preview isolada não é essa ação.
func (s *spyPort) FetchLinkPreview(context.Context, string) (domain.LinkPreviewData, bool) {
	return domain.LinkPreviewData{}, false
}

func (s *spyPort) SendPresence(context.Context, string, domain.PresenceType) error {
	s.calls++
	return s.err
}

func (s *spyPort) SendChatPresence(context.Context, string, domain.JID, string, string) error {
	s.calls++
	return s.err
}

func (s *spyPort) SubscribePresence(context.Context, string, domain.JID) error {
	s.calls++
	return s.err
}

func (s *spyPort) IsOnWhatsApp(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
	s.calls++
	return nil, s.err
}

func (s *spyPort) GetUserInfo(context.Context, string, []domain.JID) ([]domain.UserInfo, error) {
	s.calls++
	return nil, s.err
}

func (s *spyPort) GetAllContacts(context.Context, string) ([]domain.Contact, int, error) {
	s.calls++
	return nil, 0, s.err
}

func (s *spyPort) GetProfilePicture(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
	s.calls++
	return nil, s.err
}

func (s *spyPort) GetLIDForPN(context.Context, string, domain.JID) (domain.JID, error) {
	s.calls++
	return "", s.err
}

func (s *spyPort) ContactNames(context.Context, string) (map[domain.JID]domain.ContactName, error) {
	s.calls++
	return nil, s.err
}

func (s *spyPort) GroupNames(context.Context, string) (map[domain.JID]string, error) {
	s.calls++
	return nil, s.err
}

func (s *spyPort) GetPNForLID(context.Context, string, domain.JID) (domain.JID, error) {
	s.calls++
	return "", s.err
}

func (s *spyPort) GetManyLIDsForPNs(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
	s.calls++
	return nil, s.err
}

// ResolveJID espelha a regra de mapping/jid/parse.go: sem "@", aplica o
// servidor padrao; COM "@", preserva o que veio. Concatenar cegamente — como
// esta funcao fazia — produz "x@s.whatsapp.net@s.whatsapp.net" para qualquer
// entrada ja qualificada, e um dublê mais permissivo que a producao esconde
// bugs em vez de revela-los.
func (s *spyPort) ResolveJID(_ context.Context, raw string) (domain.JID, error) {
	if strings.Contains(raw, "@") {
		return domain.JID(raw), nil
	}
	return domain.JID(raw + "@s.whatsapp.net"), nil
}

func (s *spyPort) ResolveQualifiedJID(_ context.Context, raw string) (domain.JID, error) {
	if strings.Contains(raw, "@") {
		return domain.JID(raw), nil
	}
	return domain.JID(raw + "@s.whatsapp.net"), nil
}

// Métodos de appport.UserRepository — só GetQR consome esta porta, e nesse
// teste EnsureSession sempre falha antes de alcançá-los (ver doc do
// arquivo); existem apenas para satisfazer a interface em tempo de compilação.
func (s *spyPort) CreateUser(context.Context, domain.UserRecord) (bool, error) {
	s.calls++
	return false, s.err
}

func (s *spyPort) UserExists(context.Context, string) (bool, error) {
	s.calls++
	return false, s.err
}

func (s *spyPort) UpdateUser(context.Context, string, domain.UserUpdate) error {
	s.calls++
	return s.err
}

func (s *spyPort) ListUsers(context.Context, string) ([]domain.UserListEntry, error) {
	s.calls++
	return nil, s.err
}

func (s *spyPort) DeleteUser(context.Context, string) (bool, error) {
	s.calls++
	return false, s.err
}

func (s *spyPort) SessionStatus(context.Context, string) (bool, bool) {
	s.calls++
	return false, false
}

// silentLogger satisfaz appport.Logger sem poluir a saida do teste.
type silentLogger struct{}

func (silentLogger) Info(context.Context, string, ...any)  {}
func (silentLogger) Warn(context.Context, string, ...any)  {}
func (silentLogger) Error(context.Context, string, ...any) {}

// boundaryUser e o valor que o middleware de autenticacao guarda no contexto.
type boundaryUser struct{ id string }

func (u boundaryUser) Get(key string) string {
	if key == "Id" {
		return u.id
	}
	return ""
}

func withUser(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, boundaryUser{id: id}))
}

// envelope decodifica o envelope unico do ADR-002.
type envelope struct {
	Code    int             `json:"code"`
	Success bool            `json:"success"`
	Error   json.RawMessage `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v (corpo: %s)", err, rec.Body.String())
	}
	return env
}

// assertErrorEnvelope trava as tres propriedades que o ADR-002 promete para
// QUALQUER erro: status HTTP, campo code espelhando o status, success=false.
func assertErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status: got %d, want %d (corpo: %s)", rec.Code, wantStatus, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q, want application/json", ct)
	}
	env := decodeEnvelope(t, rec)
	if env.Code != wantStatus {
		t.Fatalf("envelope.code: got %d, want %d", env.Code, wantStatus)
	}
	if env.Success {
		t.Fatal("envelope.success=true numa resposta de erro")
	}
	if len(env.Error) == 0 {
		t.Fatal("envelope sem campo error numa resposta de erro")
	}
	if len(env.Data) != 0 {
		t.Fatalf("resposta de erro carregou data: %s", env.Data)
	}
}

// boundaryCase descreve um handler e o corpo minimo que ele aceita.
type boundaryCase struct {
	name string
	// build devolve o handler ja ligado a spy.
	build func(spy *spyPort) http.Handler
	// method e path da rota real.
	method string
	path   string
	// readsBody indica se o handler decodifica JSON do corpo. Handlers que
	// nao leem corpo nao podem ser testados com JSON malformado.
	readsBody bool
}

func boundaryCases() []boundaryCase {
	log := silentLogger{}
	return []boundaryCase{
		{
			name: "SendMessage",
			build: func(s *spyPort) http.Handler {
				return NewSendMessageHandler(message.NewSendMessageUseCase(s, s, s, log))
			},
			method:    http.MethodPost,
			path:      "/chat/send/text",
			readsBody: true,
		},
		{
			name: "DeleteMessage",
			build: func(s *spyPort) http.Handler {
				return NewDeleteMessageHandler(message.NewDeleteMessageUseCase(s, s, log))
			},
			method:    http.MethodPost,
			path:      "/chat/delete/message",
			readsBody: true,
		},
		{
			name: "SendEditMessage",
			build: func(s *spyPort) http.Handler {
				return NewSendEditMessageHandler(message.NewSendEditMessageUseCase(s, s, log))
			},
			method:    http.MethodPost,
			path:      "/chat/send/edit",
			readsBody: true,
		},
		{
			name: "SendTemplate",
			build: func(s *spyPort) http.Handler {
				return NewSendTemplateHandler(message.NewSendTemplateUseCase(s, s, log))
			},
			method:    http.MethodPost,
			path:      "/chat/send/template",
			readsBody: true,
		},
		{
			name:      "React",
			build:     func(s *spyPort) http.Handler { return NewReactHandler(message.NewReactUseCase(s, s, log)) },
			method:    http.MethodPost,
			path:      "/chat/react",
			readsBody: true,
		},
		{
			name:      "MarkRead",
			build:     func(s *spyPort) http.Handler { return NewMarkReadHandler(message.NewMarkReadUseCase(s, s, log)) },
			method:    http.MethodPost,
			path:      "/chat/markread",
			readsBody: true,
		},
		{
			name:      "SendPresence",
			build:     func(s *spyPort) http.Handler { return NewSendPresenceHandler(message.NewSendPresenceUseCase(s, log)) },
			method:    http.MethodPost,
			path:      "/user/presence",
			readsBody: true,
		},
		{
			name: "ChatPresence",
			build: func(s *spyPort) http.Handler {
				return NewChatPresenceHandler(message.NewChatPresenceUseCase(s, s, log))
			},
			method:    http.MethodPost,
			path:      "/chat/presence",
			readsBody: true,
		},
		{
			name: "SubscribePresence",
			build: func(s *spyPort) http.Handler {
				return NewSubscribePresenceHandler(message.NewSubscribePresenceUseCase(s, s, log))
			},
			method:    http.MethodPost,
			path:      "/user/presence/subscribe",
			readsBody: true,
		},
		{
			name:      "GetUserInfo",
			build:     func(s *spyPort) http.Handler { return NewGetUserInfoHandler(user.NewGetUserUseCase(s, s, log)) },
			method:    http.MethodPost,
			path:      "/user/info",
			readsBody: true,
		},
		{
			name:      "GetContacts",
			build:     func(s *spyPort) http.Handler { return NewGetContactsHandler(user.NewGetContactsUseCase(s, log)) },
			method:    http.MethodGet,
			path:      "/user/contacts",
			readsBody: false,
		},
		{
			name:      "GetStatus",
			build:     func(s *spyPort) http.Handler { return NewGetStatusHandler(session.NewGetStatusUseCase(s, s, log)) },
			method:    http.MethodGet,
			path:      "/session/status",
			readsBody: false,
		},
		{
			name:      "GetQR",
			build:     func(s *spyPort) http.Handler { return NewGetQRHandler(log, boundaryPairingRegistry(s)) },
			method:    http.MethodGet,
			path:      "/session/qr?engine=noise",
			readsBody: false,
		},
		{
			name:      "Disconnect",
			build:     func(s *spyPort) http.Handler { return NewDisconnectHandler(session.NewDisconnectUseCase(s, log)) },
			method:    http.MethodPost,
			path:      "/session/disconnect",
			readsBody: false,
		},
		{
			name:      "Logout",
			build:     func(s *spyPort) http.Handler { return NewLogoutHandler(session.NewLogoutUseCase(s, s, log)) },
			method:    http.MethodPost,
			path:      "/session/logout",
			readsBody: false,
		},
		{
			name:      "PairPhone",
			build:     func(s *spyPort) http.Handler { return NewPairPhoneHandler(log, boundaryPairingRegistry(s)) },
			method:    http.MethodPost,
			path:      "/session/pairphone",
			readsBody: true,
		},
		{
			name: "SetStatusMessage",
			build: func(s *spyPort) http.Handler {
				return NewSetStatusMessageHandler(session.NewSetStatusMessageUseCase(s, log))
			},
			method:    http.MethodPost,
			path:      "/session/statusmessage",
			readsBody: true,
		},
	}
}

// TestHandlers_RejectRequestWithoutUserInfo: sem o valor que o middleware
// injeta, nenhum handler pode chegar ao use case. Um handler novo que esqueca
// essa checagem entra na tabela e falha aqui.
func TestHandlers_RejectRequestWithoutUserInfo(t *testing.T) {
	for _, tc := range boundaryCases() {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyPort{}
			rec := httptest.NewRecorder()

			tc.build(spy).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			if spy.calls != 0 {
				t.Fatalf("requisicao nao autenticada alcancou a porta %d vez(es)", spy.calls)
			}
		})
	}
}

// TestHandlers_RejectWrongTypeInUserInfo: o contexto carrega `any`. Se alguem
// guardar ali um valor que nao satisfaz userInfo, o handler tem que responder
// 401 — nao entrar em panico e nao seguir com ID vazio.
func TestHandlers_RejectWrongTypeInUserInfo(t *testing.T) {
	for _, tc := range boundaryCases() {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyPort{}
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			r = r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, "isto-nao-e-userinfo"))

			tc.build(spy).ServeHTTP(rec, r)

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			if spy.calls != 0 {
				t.Fatalf("valor de contexto invalido alcancou a porta %d vez(es)", spy.calls)
			}
		})
	}
}

// TestHandlers_RejectEmptySessionID: autenticado porem sem Id e' 400, nao 401
// e nao 200. E' a distincao entre "nao sei quem voce e'" e "sei quem voce e',
// mas nao ha sessao a que essa chamada se refira".
func TestHandlers_RejectEmptySessionID(t *testing.T) {
	for _, tc := range boundaryCases() {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyPort{}
			rec := httptest.NewRecorder()

			tc.build(spy).ServeHTTP(rec, withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")), ""))

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if spy.calls != 0 {
				t.Fatalf("requisicao sem session id alcancou a porta %d vez(es)", spy.calls)
			}
		})
	}
}

// TestHandlers_RejectMalformedBody: corpo ilegivel e' 400 do cliente, e o
// handler nao pode ter falado com o WhatsApp antes de descobrir isso.
func TestHandlers_RejectMalformedBody(t *testing.T) {
	for _, tc := range boundaryCases() {
		if !tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyPort{}
			rec := httptest.NewRecorder()
			body := strings.NewReader(`{"phone": "551199`) // JSON truncado

			tc.build(spy).ServeHTTP(rec, withUser(httptest.NewRequest(tc.method, tc.path, body), "user-1"))

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if spy.calls != 0 {
				t.Fatalf("corpo malformado alcancou a porta %d vez(es)", spy.calls)
			}
		})
	}
}

// TestHandlers_ErrorEnvelopeCarriesOnlyGenericText e a garantia de sec/F16
// medida na fronteira REAL, com o handler e o use case no caminho: qualquer
// que seja o erro que a porta produza, o corpo carrega exatamente o texto
// generico do status — nunca o texto do erro.
//
// A primeira versao deste teste so checava a AUSENCIA de um segredo plantado
// na porta, e sobreviveu a mutacao `envelope["error"] = err.Error()`: os use
// cases traduzem o erro da porta para fmt.Errorf("no session") antes da
// fronteira, entao o segredo nunca chegava la e a assercao nao media nada.
// Assertar o valor EXATO mata a mutacao; a checagem do segredo fica como
// segunda linha, agora explicita sobre o que ela cobre.
func TestHandlers_ErrorEnvelopeCarriesOnlyGenericText(t *testing.T) {
	const segredo = "pq: senha do banco = hunter2 em /var/lib/pg"

	generico := map[int]string{
		http.StatusBadRequest:          "bad request",
		http.StatusUnauthorized:        "unauthorized",
		http.StatusInternalServerError: "internal server error",
	}

	for _, tc := range boundaryCases() {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyPort{err: &leakyErr{segredo}}
			rec := httptest.NewRecorder()

			// Corpo VÁLIDO, e não `{}`: desde a F66 as validações devolvem
			// apperr com CategoryValidation, e um corpo vazio faz o handler
			// responder 400 ANTES de chegar à porta — o teste passaria sem
			// nunca exercitar o caminho do erro interno que ele existe para
			// vigiar.
			//
			// Antes da F66 ele já não chegava lá: a validação crua produzia a
			// MESMA string genérica que o erro vazador produziria, então o
			// teste passava por coincidência. A conversão só tornou isso
			// visível.
			tc.build(spy).ServeHTTP(rec, withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader(corpoValidoParaFronteira)), "user-1"))

			if rec.Code < 400 {
				t.Fatalf("porta em erro produziu status de sucesso %d", rec.Code)
			}
			want, ok := generico[rec.Code]
			if !ok {
				t.Fatalf("status %d fora da taxonomia observada do ADR-002", rec.Code)
			}

			env := decodeEnvelope(t, rec)

			// After F236, boundary sentinels (errDecodePayload, errUnauthorized,
			// etc.) are *apperr.AppError. RespondJSON renders them as a structured
			// {"code":..., "message":...} object instead of a generic string.
			// Both formats are safe — apperr messages are curated, never internal
			// details. Accept either shape; the real invariant is that the leaky
			// error text never reaches the client.
			var gotStr string
			if err := json.Unmarshal(env.Error, &gotStr); err != nil {
				var gotObj struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				}
				if err2 := json.Unmarshal(env.Error, &gotObj); err2 != nil {
					t.Fatalf("envelope.error is neither a string nor a structured error: %s", env.Error)
				}
				if gotObj.Code == "" {
					t.Fatalf("structured error has empty code: %s", env.Error)
				}
				if strings.Contains(gotObj.Message, "hunter2") || strings.Contains(gotObj.Message, "/var/lib/pg") {
					t.Fatalf("detalhe interno vazou na mensagem do apperr: %s", env.Error)
				}
			} else if gotStr != want {
				t.Fatalf("envelope.error: got %q, want %q — o texto do erro interno chegou ao cliente", gotStr, want)
			}

			if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), "/var/lib/pg") {
				t.Fatalf("detalhe interno vazou no corpo: %s", rec.Body.String())
			}
		})
	}
}

type leakyErr struct{ msg string }

func (e *leakyErr) Error() string { return e.msg }

// TestSessionUser_AndInlineGuard_AgreeOnEveryInput trava a equivalencia das
// DUAS formas de fazer a mesma checagem que convivem no pacote: o helper
// sessionUser (handler_session.go) e o bloco inline repetido nos handlers de
// mensagem. Enquanto as duas existirem, elas precisam decidir igual.
func TestSessionUser_AndInlineGuard_AgreeOnEveryInput(t *testing.T) {
	log := silentLogger{}

	inputs := []struct {
		name string
		mut  func(*http.Request) *http.Request
	}{
		{"sem userinfo", func(r *http.Request) *http.Request { return r }},
		{"userinfo com id vazio", func(r *http.Request) *http.Request { return withUser(r, "") }},
		{"userinfo com tipo errado", func(r *http.Request) *http.Request {
			return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
		}},
	}

	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			// Via helper sessionUser.
			viaHelper := httptest.NewRecorder()
			NewMarkReadHandler(message.NewMarkReadUseCase(&spyPort{}, &spyPort{}, log)).
				ServeHTTP(viaHelper, in.mut(httptest.NewRequest(http.MethodPost, "/chat/markread", strings.NewReader("{}"))))

			// Via bloco inline.
			viaInline := httptest.NewRecorder()
			NewSendMessageHandler(message.NewSendMessageUseCase(&spyPort{}, &spyPort{}, &spyPort{}, log)).
				ServeHTTP(viaInline, in.mut(httptest.NewRequest(http.MethodPost, "/chat/send/text", strings.NewReader("{}"))))

			if viaHelper.Code != viaInline.Code {
				t.Fatalf("as duas formas da mesma checagem divergiram: sessionUser=%d, inline=%d",
					viaHelper.Code, viaInline.Code)
			}
		})
	}
}

// TestHandlers_AppErrFromPortReachesTheClient trava o CONSERTO do defeito que
// a versao anterior deste teste documentava.
//
// pkg/infra/wa-noise.ErrNoSession produz um *apperr.AppError com
// Code="no_session" e Category=validation (=> 400). Ate' a F11, todos os use
// cases de session/ traduziam esse erro com fmt.Errorf("no session") SEM %w:
// o wrap se perdia, RespondJSON nao conseguia errors.As, e o cliente recebia
// 500 generico em vez do 400 com error.code que o ADR-002 promete.
//
// A F11 migrou os sitios para `return err`. O erro tipado agora atravessa o
// use case intacto, e o que se assere aqui e' o Code — nao o texto.
func TestHandlers_AppErrFromPortReachesTheClient(t *testing.T) {
	spy := &spyPort{err: wasession.ErrNoSession("user-1", nil)}
	rec := httptest.NewRecorder()

	NewGetStatusHandler(session.NewGetStatusUseCase(spy, spy, silentLogger{})).
		ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/session/status", nil), "user-1"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, quero 400 — a categoria validation do apperr deixou de chegar "+
			"a' fronteira; algum use case voltou a traduzir o erro da porta", rec.Code)
	}

	env := decodeEnvelope(t, rec)
	var errObj map[string]any
	if err := json.Unmarshal(env.Error, &errObj); err != nil {
		t.Fatalf("envelope.error nao e' o objeto tipado do ADR-002: %s", env.Error)
	}
	if got := errObj["code"]; got != "no_session" {
		t.Errorf("error.code = %v, quero %q", got, "no_session")
	}
}

// corpoValidoParaFronteira reúne todos os campos que as validações de use case
// exigem, para que QUALQUER rota da tabela passe da validação e alcance a
// porta.
//
// Campos a mais são ignorados na desserialização, então um corpo só serve as
// dezessete rotas — e um corpo por rota seria uma segunda tabela para divergir
// da primeira.
//
// `Buttons` entrou com o CAP-15: sem ele /chat/send/template passaria a ser
// recusado na validação e nunca alcançaria a porta, que é exatamente a
// armadilha que o comentário do teste acima descreve — o caso passaria verde
// sem exercitar o caminho do erro interno que ele existe para vigiar.
const corpoValidoParaFronteira = `{
  "phone": "5511999999999",
  "ChatPhone": "5511999999999",
  "SenderPhone": "5511999999999",
  "id": "3EB0C767D26B8A3F1B0F",
  "url": "https://example.invalid/a.bin",
  "body": "texto",
  "content": "texto",
  "desc": "descricao",
  "name": "nome",
  "State": "available",
  "latitude": -23.5,
  "longitude": -46.6,
  "image": "data:image/png;base64,AAAA",
  "video": "data:video/mp4;base64,AAAA",
  "audio": "data:audio/ogg;base64,AAAA",
  "document": "data:application/pdf;base64,AAAA",
  "sticker": "data:image/webp;base64,AAAA",
  "vcard": "BEGIN:VCARD\nEND:VCARD",
  "group": "120363000000000000",
  "header": "cabecalho",
  "footer": "rodape",
  "file_name": "a.pdf",
  "options": ["um", "dois"],
  "buttons": [{"display_text": "Sim", "type": "quickreply"}]
}`
