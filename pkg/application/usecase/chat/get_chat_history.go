package chat

import (
	"context"
	"sort"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// chatIndexJID is the reserved value of `chat_jid` that asks for the LISTING of
// chats instead of the messages of one chat. It is part of the public contract
// recovered from commit 3dafae0 (handlers.go:5012), not an internal marker.
const chatIndexJID = "index"

// defaultHistoryLimit is the number of messages returned when the caller sends
// no `limit`. 50, as in the historical handler.
const defaultHistoryLimit = 50

// lidSuffix is the server part of a @lid identifier. Named because it is
// matched here and would otherwise be a bare literal on a decision that
// changes what the caller sees (ADR-0004).
const lidSuffix = "@lid"

// ChatHistoryQuery is the parsed form of the two query parameters GET
// /chat/history accepts.
//
// Limit is a pointer so that "absent" and "explicitly 0" stay distinguishable:
// absent means the default of 50, and an explicit 0 was accepted by the
// historical handler and returned nothing. Collapsing them into an int would
// silently change the second case.
//
// No offset, before, after, message_id, fromMe or media-type filter: none of
// them ever existed on this endpoint, and adding one here would be inventing
// contract rather than recovering it.
type ChatHistoryQuery struct {
	ChatJID string
	Limit   *int
}

// ChatHistoryResult carries exactly one of the two branches. Messages is set
// for a normal chat read; Index is set for chat_jid=index. The two shapes are
// different on the wire and always were.
type ChatHistoryResult struct {
	Messages []appport.ChatHistoryMessage
	Index    map[string][]appport.ChatIndexEntry
	IsIndex  bool
}

// GetChatHistoryUseCase reads the LOCAL message history of a chat.
//
// It deliberately does NOT call EnsureSession. History is a read of our own
// database, gated by the user's `history` setting; requiring a live WhatsApp
// transport would make a purely local read fail whenever the phone is offline.
// This is the difference from the download use cases, which genuinely need the
// session.
type GetChatHistoryUseCase struct {
	history appport.ChatHistoryReader
	// lids resolves a @lid to its phone JID. NIL IS VALID and means "no
	// translation": the read then behaves exactly as it did before F183.
	lids   appport.LIDResolver
	logger appport.Logger
}

// NewGetChatHistoryUseCase creates the use case without LID translation.
//
// Kept so every existing caller and test keeps compiling and keeps its exact
// behaviour. Translation is opt-in through WithLIDResolver.
func NewGetChatHistoryUseCase(h appport.ChatHistoryReader, l appport.Logger) *GetChatHistoryUseCase {
	return &GetChatHistoryUseCase{history: h, logger: l}
}

// WithLIDResolver returns the use case with @lid translation enabled.
func (uc *GetChatHistoryUseCase) WithLIDResolver(r appport.LIDResolver) *GetChatHistoryUseCase {
	uc.lids = r
	return uc
}

// Execute runs the history gate and then the read.
//
// cachedHistory is the `History` field of the cached userinfo, already parsed.
// The gate is two-step ON PURPOSE and must not be collapsed into one check:
// the cached value can be stale (the cache is keyed by token and has a TTL),
// so a 0 is REVALIDATED against the users table before the request is refused.
// Only a 0 that survives revalidation produces 501 — a feature the user turned
// off, which is what 501 has meant on this endpoint since 3dafae0.
func (uc *GetChatHistoryUseCase) Execute(ctx context.Context, userID string, cachedHistory int, q ChatHistoryQuery) (*ChatHistoryResult, error) {
	if cachedHistory == 0 {
		fresh, err := uc.history.HistoryLimit(ctx, userID)
		if err != nil {
			uc.logger.Error(ctx, "failed to revalidate history setting", "txtID", userID, "error", err)
			return nil, apperr.New("history_revalidation_failed", apperr.CategoryInternal,
				"failed to read history configuration", true, err)
		}
		if fresh == 0 {
			uc.logger.Warn(ctx, "chat history read refused: message history is disabled", "txtID", userID)
			return nil, apperr.New("history_disabled", apperr.CategoryNotImplemented,
				"message history is disabled for this user", false, nil)
		}
		uc.logger.Info(ctx, "history setting revalidated from persistence", "txtID", userID)
	}

	if q.ChatJID == "" {
		uc.logger.Warn(ctx, "chat history read refused: chat_jid is required", "txtID", userID)
		return nil, apperr.New("missing_chat_jid", apperr.CategoryValidation,
			"chat_jid is required", false, nil)
	}

	if q.ChatJID == chatIndexJID {
		index, err := uc.history.ChatIndexByUser(ctx, userID)
		if err != nil {
			uc.logger.Error(ctx, "failed to read chat index", "txtID", userID, "error", err)
			return nil, apperr.New("chat_index_failed", apperr.CategoryInternal,
				"failed to get chat mappings", true, err)
		}
		if index == nil {
			index = map[string][]appport.ChatIndexEntry{}
		}
		return &ChatHistoryResult{Index: index, IsIndex: true}, nil
	}

	limit := defaultHistoryLimit
	if q.Limit != nil {
		limit = *q.Limit
	}

	messages, err := uc.history.ListChatMessages(ctx, userID, q.ChatJID, limit)
	if err != nil {
		uc.logger.Error(ctx, "failed to read chat history", "txtID", userID, "error", err)
		return nil, apperr.New("chat_history_failed", apperr.CategoryInternal,
			"failed to get message history", true, err)
	}

	messages = uc.mergeAlternateKey(ctx, userID, q.ChatJID, limit, messages)

	if messages == nil {
		messages = []appport.ChatHistoryMessage{}
	}
	return &ChatHistoryResult{Messages: messages}, nil
}

// mergeAlternateKey adds the messages stored under the OTHER identifier of the
// same conversation, when the caller asked with a @lid.
//
// WHY THIS EXISTS (HOUSEKEEP F183). Measured on 30 real conversations: the
// caller lists chats, gets @lid for every one of them, asks for the history of
// each with the identifier the listing just handed over — and 27 of 30 come
// back EMPTY, because those rows are stored under the phone JID. Nothing is
// lost; it is filed under the other name.
//
// WHY IT MERGES INSTEAD OF REPLACING. Three of those conversations have rows
// under BOTH keys. Translating and then reading only the phone key would swap
// an empty answer for HALF an answer, which is worse: an empty list at least
// looks wrong.
//
// WHY IT DEGRADES INSTEAD OF FAILING. This use case deliberately does not
// require a live session — history is a read of our own database and must work
// with the phone offline (see the type comment). The resolver, however, goes
// through the session-bound client. So a resolution that fails is NOT an error
// here: the caller gets exactly what it got before this function existed. The
// read is never worse than it was, only sometimes better.
//
// WHY ONLY @lid -> PN, and not the reverse: only that direction was measured
// to fail in the field. The reverse would be inventing a fix for a defect
// nobody has seen, which is how this repository grows contract by accident.
func (uc *GetChatHistoryUseCase) mergeAlternateKey(
	ctx context.Context, userID, chatJID string, limit int, primary []appport.ChatHistoryMessage,
) []appport.ChatHistoryMessage {
	if uc.lids == nil || !strings.HasSuffix(chatJID, lidSuffix) {
		return primary
	}

	pn, err := uc.lids.GetPNForLID(ctx, userID, domain.JID(chatJID))
	if err != nil {
		// Warn, and the port has no Debug, so the choice is between this and
		// silence. Warn is right: the caller just got FEWER rows than exist,
		// and an operator chasing "why is this chat empty" needs the line.
		//
		// It is not the F180 kind of noise — the resolver only fails when the
		// session is down, which is not the normal path for a read that the
		// caller expects to answer.
		uc.logger.Warn(ctx, "chat history: lid not translated, returning stored rows only",
			"txtID", userID, "chatJID", chatJID, "error", err)
		return primary
	}
	if pn == "" || string(pn) == chatJID {
		return primary
	}

	alternate, err := uc.history.ListChatMessages(ctx, userID, string(pn), limit)
	if err != nil {
		uc.logger.Warn(ctx, "chat history: alternate key read failed, returning stored rows only",
			"txtID", userID, "chatJID", chatJID, "error", err)
		return primary
	}
	if len(alternate) == 0 {
		return primary
	}

	// A fusão vive AQUI, e não numa função à parte, por uma razão que vale
	// escrever: como função separada ela é elegível para o gate de log e não
	// tem nada que valha registar — é aritmética pura, sem modo de falha. A
	// escolha era inventar um log para mover um número, pendurar uma isenção,
	// ou reconhecer que a fusão não é uma operação independente. É a terceira:
	// quem tem algo a dizer ao operador é esta função, que já o diz abaixo com
	// os números das duas chaves.
	//
	// Deduplica por message_id porque a mesma mensagem PODE aparecer sob as
	// duas chaves: o índice único é (user_id, message_id), o que impede a
	// mesma mensagem de ser gravada duas vezes para um utilizador, mas NÃO
	// impede a mesma conversa de estar arquivada sob dois chat_jid. Ler o
	// índice como se garantisse ausência de duplicados na fusão seria
	// atribuir-lhe uma garantia que ele nunca deu.
	vistos := make(map[string]struct{}, len(primary)+len(alternate))
	merged := make([]appport.ChatHistoryMessage, 0, len(primary)+len(alternate))
	for _, origem := range [][]appport.ChatHistoryMessage{primary, alternate} {
		for _, m := range origem {
			if _, dup := vistos[m.MessageID]; dup {
				continue
			}
			vistos[m.MessageID] = struct{}{}
			merged = append(merged, m)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Timestamp.After(merged[j].Timestamp) })
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}

	uc.logger.Info(ctx, "chat history: merged rows stored under the phone JID",
		"txtID", userID, "chatJID", chatJID,
		"underLID", len(primary), "underPN", len(alternate), "returned", len(merged))

	return merged
}
