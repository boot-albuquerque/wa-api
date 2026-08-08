package contractsfake

import (
	"context"
	"time"

	port "wa-api/pkg/application/contracts"
)

// --- ChatActivityReader --------------------------------------------------

// ChatActivityReaderGetLastActivityByUserCall é uma chamada a
// GetLastActivityByUser.
type ChatActivityReaderGetLastActivityByUserCall struct {
	Ctx    context.Context
	UserID string
}

// ChatActivityReader é o fake de port.ChatActivityReader.
type ChatActivityReader struct {
	GetChatPushNamesFunc  func(ctx context.Context, userID string) (map[string]string, error)
	GetChatPushNamesCalls int

	GetLastActivityByUserFunc  func(ctx context.Context, userID string) (map[string]time.Time, error)
	GetLastActivityByUserCalls []ChatActivityReaderGetLastActivityByUserCall
}

// GetChatPushNames implementa port.ChatActivityReader. Zero-value devolve
// mapa vazio, que e' o historico de quem ainda nao recebeu mensagem nenhuma.
func (f *ChatActivityReader) GetChatPushNames(ctx context.Context, userID string) (map[string]string, error) {
	f.GetChatPushNamesCalls++
	if f.GetChatPushNamesFunc != nil {
		return f.GetChatPushNamesFunc(ctx, userID)
	}
	return map[string]string{}, nil
}

var _ port.ChatActivityReader = (*ChatActivityReader)(nil)

// GetLastActivityByUser implementa port.ChatActivityReader.
func (f *ChatActivityReader) GetLastActivityByUser(ctx context.Context, userID string) (map[string]time.Time, error) {
	f.GetLastActivityByUserCalls = append(f.GetLastActivityByUserCalls, ChatActivityReaderGetLastActivityByUserCall{Ctx: ctx, UserID: userID})
	if f.GetLastActivityByUserFunc != nil {
		return f.GetLastActivityByUserFunc(ctx, userID)
	}
	return nil, nil
}
