package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Fase 12 — /user/presence, /user/presence/subscribe, /chat/presence,
// /chat/markread e /chat/react.
//
// Os cinco compartilham a forma sessionUser -> decode -> use case, e diferem
// so' na porta que consomem. A tabela injeta falha em CADA degrau: guarda de
// sessao do whatsmeow, resolucao de JID, e a operacao em si — porque um
// handler que colapsasse os tres num unico 500 sem causa continuaria passando
// num teste que so' olhasse o status.

// presenceDeps reune as portas que os cinco handlers consomem.
type presenceDeps struct {
	presence  *contractsfake.PresenceController
	messenger *contractsfake.ChatMessenger
	jids      *contractsfake.JIDResolver
}

func newPresenceDeps() *presenceDeps {
	return &presenceDeps{
		presence:  &contractsfake.PresenceController{},
		messenger: &contractsfake.ChatMessenger{},
		jids:      &contractsfake.JIDResolver{},
	}
}

// failSession recusa a sessao nas duas portas — cada caso usa uma so', e
// configurar ambas mantem a tabela sem ramo.
func (d *presenceDeps) failSession(err error) {
	d.presence.SessionGuard = contractsfake.FailSession(err)
	d.messenger.SessionGuard = contractsfake.FailSession(err)
}

func (d *presenceDeps) failJID(err error) {
	d.jids.ResolveJIDFunc = func(context.Context, string) (domain.JID, error) { return "", err }
}

// presenceCase descreve um handler de presenca/reacao.
type presenceCase struct {
	name string
	path string
	// build liga o handler as portas fake.
	build func(d *presenceDeps) http.Handler
	// validBody e' o menor corpo aceito pelo use case.
	validBody string
	// emptyBodyErr e' a causa produzida pelo corpo `{}`.
	emptyBodyErr string
	// jidErr e' a causa produzida quando ResolveJID falha. Vazio quando o
	// handler nao resolve JID (SendPresence).
	jidErr string
	// failOp injeta falha na operacao final da porta.
	failOp func(d *presenceDeps, err error)
	// opErr e' a causa que o use case produz a partir dessa falha.
	opErr string
}

func presenceCases() []presenceCase {
	log := &contractsfake.Logger{}
	return []presenceCase{
		{
			name: "SendPresence",
			path: "/user/presence",
			build: func(d *presenceDeps) http.Handler {
				return NewSendPresenceHandler(message.NewSendPresenceUseCase(d.presence, log))
			},
			validBody:    `{"type":"available"}`,
			emptyBodyErr: "invalid presence type",
			failOp: func(d *presenceDeps, err error) {
				d.presence.SendPresenceFunc = func(context.Context, string, domain.PresenceType) error { return err }
			},
			opErr: "failure sending presence",
		},
		{
			name: "SubscribePresence",
			path: "/user/presence/subscribe",
			build: func(d *presenceDeps) http.Handler {
				return NewSubscribePresenceHandler(message.NewSubscribePresenceUseCase(d.presence, d.jids, log))
			},
			validBody:    `{"Phone":"5511999999999"}`,
			emptyBodyErr: "missing Phone in Payload",
			jidErr:       "could not parse Phone",
			failOp: func(d *presenceDeps, err error) {
				d.presence.SubscribePresenceFunc = func(context.Context, string, domain.JID) error { return err }
			},
			opErr: "failure subscribing to presence",
		},
		{
			name: "ChatPresence",
			path: "/chat/presence",
			build: func(d *presenceDeps) http.Handler {
				return NewChatPresenceHandler(message.NewChatPresenceUseCase(d.presence, d.jids, log))
			},
			validBody:    `{"Phone":"5511999999999","State":"composing"}`,
			emptyBodyErr: "missing Phone in Payload",
			jidErr:       "could not parse Phone",
			failOp: func(d *presenceDeps, err error) {
				d.presence.SendChatPresenceFunc = func(context.Context, string, domain.JID, string, string) error { return err }
			},
			opErr: "failure sending chat presence",
		},
		{
			name: "MarkRead",
			path: "/chat/markread",
			build: func(d *presenceDeps) http.Handler {
				return NewMarkReadHandler(message.NewMarkReadUseCase(d.messenger, d.jids, log))
			},
			validBody:    `{"Id":["MSG1"],"ChatPhone":"5511999999999"}`,
			emptyBodyErr: "missing ChatPhone in Payload",
			jidErr:       "could not parse ChatPhone",
			failOp: func(d *presenceDeps, err error) {
				d.messenger.MarkReadFunc = func(context.Context, string, []string, time.Time, domain.JID, domain.JID) error {
					return err
				}
			},
			opErr: "failure marking messages as read",
		},
		{
			name: "React",
			path: "/chat/react",
			build: func(d *presenceDeps) http.Handler {
				return NewReactHandler(message.NewReactUseCase(d.messenger, d.jids, log))
			},
			validBody:    `{"Phone":"5511999999999","Body":"ok","Id":"MSG1"}`,
			emptyBodyErr: "missing Phone in Payload",
			jidErr:       "could not parse Phone",
			failOp: func(d *presenceDeps, err error) {
				d.messenger.SendReactionFunc = func(context.Context, string, domain.JID, domain.Reaction) (domain.MessageSendResult, error) {
					return domain.MessageSendResult{}, err
				}
			},
			opErr: "error sending message",
		},
	}
}

// TestPresenceHandlers_Success: 200 e nenhum registro de caminho de saida.
func TestPresenceHandlers_Success(t *testing.T) {
	for _, tc := range presenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if env := decodeEnvelope(t, rec); !env.Success {
				t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
			}
			ipmAssertNoOutcomeLog(t, recs)
		})
	}
}

// TestPresenceHandlers_Unauthorized e TestPresenceHandlers_MissingSessionID
// cobrem os dois 4xx produzidos por sessionUser (handler_session.go). O
// registro sai de la', mas quem o exercita por estas cinco rotas e' este
// arquivo — e o co-gate D vale igual, venha o log de onde vier.
func TestPresenceHandlers_Unauthorized(t *testing.T) {
	for _, tc := range presenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, tc.validBody, nil)

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, recs, "unauthorized")
			if len(d.presence.EnsureSessionCalls)+len(d.messenger.EnsureSessionCalls) != 0 {
				t.Fatal("requisicao nao autenticada alcancou a porta")
			}
		})
	}
}

func TestPresenceHandlers_MissingSessionID(t *testing.T) {
	for _, tc := range presenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, "missing session id")
			if len(d.presence.EnsureSessionCalls)+len(d.messenger.EnsureSessionCalls) != 0 {
				t.Fatal("requisicao sem session id alcancou a porta")
			}
		})
	}
}

// TestPresenceHandlers_MalformedBody: 400 logado com a causa do decode.
func TestPresenceHandlers_MalformedBody(t *testing.T) {
	for _, tc := range presenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, `{"Phone":`,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, "could not decode payload")
			if len(d.presence.EnsureSessionCalls)+len(d.messenger.EnsureSessionCalls) != 0 {
				t.Fatal("corpo malformado alcancou a porta")
			}
		})
	}
}

// TestPresenceHandlers_IncompletePayload: JSON valido, campo obrigatorio
// ausente.
func TestPresenceHandlers_IncompletePayload(t *testing.T) {
	for _, tc := range presenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, `{}`,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, tc.emptyBodyErr)
		})
	}
}

// TestPresenceHandlers_SessionFailure: sem sessao do whatsmeow.
func TestPresenceHandlers_SessionFailure(t *testing.T) {
	for _, tc := range presenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()
			d.failSession(ipmErrBoom)

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
		})
	}
}

// TestPresenceHandlers_JIDFailure: o telefone do payload nao vira JID.
func TestPresenceHandlers_JIDFailure(t *testing.T) {
	for _, tc := range presenceCases() {
		if tc.jidErr == "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()
			d.failJID(ipmErrBoom)

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, tc.jidErr)
		})
	}
}

// TestPresenceHandlers_OperationFailure: a porta do WhatsApp recusa a
// operacao final.
func TestPresenceHandlers_OperationFailure(t *testing.T) {
	for _, tc := range presenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newPresenceDeps()
			tc.failOp(d, ipmErrBoom)

			rec, recs := ipmServe(t, tc.build(d), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, tc.opErr)
		})
	}
}
