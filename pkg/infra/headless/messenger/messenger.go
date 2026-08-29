// Package messenger adapts marking-read and reacting to the application's
// ChatMessenger port.
//
// # MarkRead works by HALVES, and which half matters depends on the caller
//
// H160 measured both sides. The LOCAL acknowledgement works and is proven on
// both accounts: unreadCount goes 1 → 0, changed=true, no error. The RECEIPT TO
// THE SENDER does not arrive — with a dual session, marking read in either
// direction leaves the other side's ack at 2 for 60 seconds, and the
// markAvailable hypothesis was tested and REFUTED.
//
// The port is called MarkRead and does not distinguish the two. So:
//
//	a caller clearing its own unread badge      is served
//	a caller expecting the sender to see ticks  is NOT, and it is measured
//
// This adapter satisfies the port because the operation it names does happen.
// The caveat lives here because there is nowhere in the signature to put it, and
// a caller that needs the second half must know it is not getting it.
package messenger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
)

// errNoTarget refuses a reaction with nothing to react to. The page needs the
// message key, and an empty one would reach it as a lookup for "".
var errNoTarget = errors.New("headless: reaction without a target message")

const (
	markLabel   = "adapter/mark-read"
	reactLabel  = "adapter/send-reaction"
	editLabel   = "adapter/edit-message"
	revokeLabel = "adapter/revoke-message"
	voteLabel   = "adapter/send-poll-vote"
)

// marker and reactor are the slices of the page capabilities this adapter uses.
type marker interface {
	MarkRead(ctx context.Context, jid, label string) (headless.MarkReadResult, error)
}

type editor interface {
	Text(ctx context.Context, msgID, newText, label string) (headless.EditResult, error)
}

type revoker interface {
	ForEveryone(ctx context.Context, msgID string, clearMedia bool, label string) (headless.RevokeResult, error)
}

type voter interface {
	Vote(ctx context.Context, messageID string, options []string, label string) error
}

type reactor interface {
	Add(ctx context.Context, msgID, emoji, label string) (headless.ReactionResult, error)
	Remove(ctx context.Context, msgID, label string) (headless.ReactionResult, error)
}

// Messenger implements appport.ChatMessenger over a headless session.
type Messenger struct {
	sessions *adapter.Sessions
	// Overridable in tests. Nil uses the real page capabilities.
	newMarker  func(ctx context.Context, txtID string) (marker, error)
	newReactor func(ctx context.Context, txtID string) (reactor, error)
	newEditor  func(ctx context.Context, txtID string) (editor, error)
	newRevoker func(ctx context.Context, txtID string) (revoker, error)
	newVoter   func(ctx context.Context, txtID string) (voter, error)
}

// NewMessenger builds the adapter.
func NewMessenger(sessions *adapter.Sessions) *Messenger { return &Messenger{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (m *Messenger) EnsureSession(ctx context.Context, txtID string) error {
	return m.sessions.EnsureSession(ctx, txtID)
}

// MarkRead marks a conversation read. See the package doc for what that does and
// does not achieve on this transport.
//
// The ids and the timestamp are IGNORED, and saying so is the point: the page
// marks the CONVERSATION, not a list of messages, and there is no per-message
// seen call to route them to. Accepting them silently and marking the whole chat
// would be a wider effect than the caller asked for, dressed as the narrow one.
func (m *Messenger) MarkRead(ctx context.Context, txtID string, _ []string, _ time.Time, chat, _ domain.JID) error {
	pageJID, err := adapter.ToPageJID(chat)
	if err != nil {
		return err
	}
	mk, err := m.marker(ctx, txtID)
	if err != nil {
		return err
	}
	// The result carries the before/after unread count — the postcondition the
	// capability reads back. It is discarded because the port promises only an
	// error; "nothing to mark" is a success, exactly as an already-archived chat
	// is.
	_, err = mk.MarkRead(ctx, pageJID, markLabel)
	return err
}

// SendReaction adds or removes a reaction.
//
// An EMPTY Text removes, which is the domain's own convention — the use case
// turns the word "remove" into an empty string before it gets here. Routing both
// through Add with an empty emoji would leave the page to guess.
func (m *Messenger) SendReaction(ctx context.Context, txtID string, _ domain.JID, r domain.Reaction) (domain.MessageSendResult, error) {
	if r.TargetMessageID == "" {
		return domain.MessageSendResult{}, errNoTarget
	}
	rc, err := m.reactor(ctx, txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	if r.Text == "" {
		if _, err := rc.Remove(ctx, r.TargetMessageID, reactLabel); err != nil {
			return domain.MessageSendResult{}, err
		}
	} else if _, err := rc.Add(ctx, r.TargetMessageID, r.Text, reactLabel); err != nil {
		return domain.MessageSendResult{}, err
	}

	// The port's result carries a timestamp. The page does not report one for a
	// reaction, so this is the moment the call returned — and NOT a claim about
	// when the server recorded it. Inventing a server timestamp would be a
	// fact-shaped guess.
	return domain.MessageSendResult{Timestamp: time.Now()}, nil
}

func (m *Messenger) marker(ctx context.Context, txtID string) (marker, error) {
	if m.newMarker != nil {
		return m.newMarker(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return headless.NewChatLister(m.sessions.Runner(), eval), nil
}

func (m *Messenger) reactor(ctx context.Context, txtID string) (reactor, error) {
	if m.newReactor != nil {
		return m.newReactor(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return headless.NewReactor(m.sessions.Runner(), eval), nil
}

// EditMessage edits one's own message.
//
// O port devolve MessageSendResult, e o que a página devolve é outra coisa:
// comprimentos antes e depois, e se o modelo foi CARIMBADO como editado —
// `Recorded`. Aplicado-mas-não-carimbado é estado real, e é o que decide se o
// cliente de quem recebe mostra "editada".
//
// Então a tradução recusa em vez de achatar: uma edição que a página não
// carimbou não passa por feita. O corpo NÃO entra em erro nenhum — é conteúdo
// do usuário, e a recusa fala em comprimento.
//
// ctxInfo é ignorado de propósito, e dito em voz alta: a capability edita pelo
// ID da mensagem e não aceita contexto de citação. Aceitar o parâmetro e
// descartá-lo em silêncio faria o chamador crer que a citação foi preservada.
func (m *Messenger) EditMessage(ctx context.Context, txtID string, _ domain.JID, messageID, newText string, ctxInfo *domain.EditContextInfo) (domain.MessageSendResult, error) {
	if messageID == "" {
		return domain.MessageSendResult{}, errNoTarget
	}
	if ctxInfo != nil {
		return domain.MessageSendResult{}, errors.New(
			"headless: a capability de edicao nao aceita contexto de citacao, e descarta-lo em silencio faria o chamador crer que foi preservado")
	}
	e, err := m.editor(ctx, txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	out, err := e.Text(ctx, messageID, newText, editLabel)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	if !out.Recorded {
		return domain.MessageSendResult{}, fmt.Errorf(
			"headless: edicao de %d para %d bytes aplicada sem o modelo a carimbar como editada; "+
				"quem recebe nao veria \"editada\"", out.FromLen, out.ToLen)
	}
	return domain.MessageSendResult{ID: messageID}, nil
}

func (m *Messenger) editor(ctx context.Context, txtID string) (editor, error) {
	if m.newEditor != nil {
		return m.newEditor(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return headless.NewEditor(m.sessions.Runner(), eval), nil
}

// RevokeMessage deletes a message for everyone.
//
// clearMedia é falso, e a escolha é do CHAMADOR na capability — este port não
// tem onde a receber. Falso preserva a mídia já baixada de quem recebeu, que é
// o comportamento menos destrutivo dos dois; escolher o outro por conta própria
// apagaria arquivo de terceiros sem ninguém ter pedido.
//
// O `As` da capability diz QUAL direito a página usou — mensagem própria, ou
// admin a remover a de outrem. São atos diferentes, e o port devolve só um id;
// então o que se pode fazer é não perder a informação em silêncio quando ela
// vier vazia, porque aí a página não disse o que fez.
func (m *Messenger) RevokeMessage(ctx context.Context, txtID string, _ domain.JID, messageID string) (domain.MessageSendResult, error) {
	if messageID == "" {
		return domain.MessageSendResult{}, errNoTarget
	}
	rv, err := m.revoker(ctx, txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	out, err := rv.ForEveryone(ctx, messageID, false, revokeLabel)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	if out.As == "" {
		return domain.MessageSendResult{}, errors.New(
			"headless: a pagina revogou sem dizer com que direito, e nao se reporta como feito o que nao se sabe que foi feito")
	}
	return domain.MessageSendResult{ID: messageID}, nil
}

func (m *Messenger) revoker(ctx context.Context, txtID string) (revoker, error) {
	if m.newRevoker != nil {
		return m.newRevoker(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return headless.NewRevoker(m.sessions.Runner(), eval), nil
}

// SendPollVote vota numa enquete que outra pessoa criou.
//
// A capability vota pelo ID da mensagem da ENQUETE, e é só disso que ela
// precisa: a página já sabe de que conversa a mensagem é. Os outros campos do
// payload — PollChat, PollSender, PollTimestamp — existem porque o transporte
// de socket tem de RECONSTRUIR a chave do voto a partir deles, e aqui não há
// chave a reconstruir.
//
// Ignorá-los é correto, e é dito em voz alta em vez de silenciado. O que NÃO
// seria correto é aceitar um payload cujo ID esteja vazio: aí a página
// procuraria "" e a resposta seria sobre outra coisa.
//
// Nota medida, e ela importa: CRIAR enquete pela headless não entrega (ack fica
// em 0, H98/H101). VOTAR é operação diferente e não herda esse defeito — a
// enquete já existe, criada por quem quer que seja. Não presumo que funcione
// por isso; presumo apenas que a medição da criação não se aplica aqui.
func (m *Messenger) SendPollVote(ctx context.Context, txtID string, _ domain.JID, payload domain.PollVotePayload, _ string) (domain.MessageSendResult, error) {
	if payload.PollMessageID == "" {
		return domain.MessageSendResult{}, errNoTarget
	}
	if len(payload.OptionNames) == 0 {
		return domain.MessageSendResult{}, errors.New(
			"headless: voto sem opcao escolhida; um voto vazio RETIRA o voto e tem de ser pedido como tal")
	}
	v, err := m.voter(ctx, txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	if err := v.Vote(ctx, payload.PollMessageID, payload.OptionNames, voteLabel); err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{ID: payload.PollMessageID}, nil
}

func (m *Messenger) voter(ctx context.Context, txtID string) (voter, error) {
	if m.newVoter != nil {
		return m.newVoter(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return headless.NewPollManager(m.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.ChatMessenger = (*Messenger)(nil)
