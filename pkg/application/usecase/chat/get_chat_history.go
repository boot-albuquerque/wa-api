package chat

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
)

// chatIndexJID is the reserved value of `chat_jid` that asks for the LISTING of
// chats instead of the messages of one chat. It is part of the public contract
// recovered from commit 3dafae0 (handlers.go:5012), not an internal marker.
const chatIndexJID = "index"

// defaultHistoryLimit is the number of messages returned when the caller sends
// no `limit`. 50, as in the historical handler.
const defaultHistoryLimit = 50

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
	logger  appport.Logger
}

// NewGetChatHistoryUseCase creates the use case.
func NewGetChatHistoryUseCase(h appport.ChatHistoryReader, l appport.Logger) *GetChatHistoryUseCase {
	return &GetChatHistoryUseCase{history: h, logger: l}
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
	if messages == nil {
		messages = []appport.ChatHistoryMessage{}
	}
	return &ChatHistoryResult{Messages: messages}, nil
}
