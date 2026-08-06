package user

import (
	"context"
	"time"

	appport "wa-api/pkg/application/contracts"
)

// GetContactsLastActivityUseCase devolve o timestamp da última mensagem por
// chat, derivado do backfill local de message_history (HistorySync
// pós-pareamento + mensagens correntes). Não exige sessão whatsmeow ativa —
// é leitura de banco local, funciona mesmo com a sessão em standby.
type GetContactsLastActivityUseCase struct {
	activity appport.ChatActivityReader
	logger   appport.Logger
}

// NewGetContactsLastActivityUseCase creates a new instance.
func NewGetContactsLastActivityUseCase(ar appport.ChatActivityReader, logger appport.Logger) *GetContactsLastActivityUseCase {
	return &GetContactsLastActivityUseCase{activity: ar, logger: logger}
}

// Execute retrieves last-activity-per-chat.
func (uc *GetContactsLastActivityUseCase) Execute(ctx context.Context, userID string) (map[string]time.Time, error) {
	result, err := uc.activity.GetLastActivityByUser(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get contacts last activity", "error", err, "user_id", userID)
		return nil, err
	}
	uc.logger.Info(ctx, "Retrieved contacts last activity", "user_id", userID, "count", len(result))
	return result, nil
}
