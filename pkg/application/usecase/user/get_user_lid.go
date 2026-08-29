package user

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetUserLIDUseCase obtém o LID para um JID
type GetUserLIDUseCase struct {
	contacts appport.IdentityResolver
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewGetUserLIDUseCase cria uma nova instância
func NewGetUserLIDUseCase(cd appport.IdentityResolver, jr appport.JIDResolver, logger appport.Logger) *GetUserLIDUseCase {
	return &GetUserLIDUseCase{contacts: cd, jids: jr, logger: logger}
}

// LIDResult representa o resultado com JID e LID
type LIDResult struct {
	JID string
	LID string
}

// Execute obtém o LID para um JID
func (uc *GetUserLIDUseCase) Execute(ctx context.Context, userID string, req domain.GetUserLIDRequest) (*LIDResult, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	// Parse JID
	jid, err := uc.jids.ResolveQualifiedJID(ctx, req.JID)
	if err != nil {
		uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "jid", req.JID)
		return nil, apperr.New("invalid_jid", apperr.CategoryValidation,
			"invalid jid format", false, err)
	}

	// F182: o TIPO do JID é validação, não falha de infraestrutura. Antes
	// disto, passar um LID a esta rota chegava ao store, que recusava com
	// "invalid GetLIDForPN call with non-PN JID", e o cliente recebia 500 —
	// "erro interno" para um erro DELE, determinístico, que repetir não
	// resolve. O 500 abaixo continua certo para o que ele cobre: falha do
	// store. Os dois caminhos é que estavam fundidos num só.
	if !jid.IsPN() {
		uc.logger.Warn(ctx, "LID lookup refused: jid is not a phone number",
			"user_id", userID, "jid", req.JID)
		return nil, apperr.New("jid_not_pn", apperr.CategoryValidation,
			"this route resolves the LID OF a phone number: pass a "+domain.ServerPN+" jid, not "+domain.ServerLID,
			false, nil)
	}

	// Get LID from store
	lid, err := uc.contacts.GetLIDForPN(ctx, userID, jid)
	if err != nil {
		// NAO e 404: aqui a PORTA falhou — o store quebrou, e o cliente nao
		// tem como saber se aquele numero tem LID ou nao. Reportar "nao
		// encontrado" faria ele PARAR de tentar diante de uma falha
		// transitoria. A mensagem antiga ("LID not found: %w") ja era
		// enganosa; o 500 e que estava acidentalmente certo.
		uc.logger.Error(ctx, "Failed to get LID", "error", err, "jid", req.JID)
		return nil, fmt.Errorf("failed to get LID: %w", err)
	}

	if lid == "" {
		return nil, apperr.New("lid_not_found", apperr.CategoryNotFound,
			"LID not found for this number", false, nil)
	}

	return &LIDResult{
		JID: string(jid),
		LID: string(lid),
	}, nil
}
