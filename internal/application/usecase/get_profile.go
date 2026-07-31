package usecase

import (
	"context"
	"encoding/json"

	"disparazap/internal/application/port"

	"go.mau.fi/whatsmeow"
)

// GetProfileUseCase implementa a lógica de obtenção de perfil WhatsApp.
type GetProfileUseCase struct {
	clientProvider    port.ClientProvider
	dataAccessFactory func(client *whatsmeow.Client) port.ProfileDataAccess
	logger            port.Logger
}

// NewGetProfileUseCase cria o usecase com dependências injetadas.
// dataAccessFactory é tipicamente whatsmeow.NewProfileDataAccess.
func NewGetProfileUseCase(
	clientProvider port.ClientProvider,
	dataAccessFactory func(client *whatsmeow.Client) port.ProfileDataAccess,
	logger port.Logger,
) *GetProfileUseCase {
	return &GetProfileUseCase{
		clientProvider:    clientProvider,
		dataAccessFactory: dataAccessFactory,
		logger:            logger,
	}
}

// ProfileResult representa o resultado do usecase.
type ProfileResult struct {
	Pushname     string `json:"pushname"`
	AvatarURL    string `json:"avatar_url"`
	AvatarID     string `json:"avatar_id"`
	JID          string `json:"jid"`
	FullName     string `json:"full_name"`
	BusinessName string `json:"business_name"`
}

// Execute obtém o perfil WhatsApp para o txtID informado.
func (uc *GetProfileUseCase) Execute(ctx context.Context, txtID string) (string, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return "", err
	}
	if client == nil {
		uc.logger.Warn("no session for txtID", "txtID", txtID)
		return "", ErrNoSession
	}

	da := uc.dataAccessFactory(client)
	result := buildProfile(ctx, da)
	responseJSON, err := json.Marshal(result)
	if err != nil {
		uc.logger.Error("failed to marshal profile", "error", err)
		return "", err
	}
	return string(responseJSON), nil
}

// buildProfile constrói o perfil a partir do ProfileDataAccess.
// Extraída para teste unitário sem depender de *whatsmeow.Client.
// Chama OwnJID() uma única vez e cacheia o resultado para reuso.
func buildProfile(ctx context.Context, da port.ProfileDataAccess) ProfileResult {
	result := ProfileResult{
		Pushname:     "",
		AvatarURL:    "",
		AvatarID:     "",
		JID:          "",
		FullName:     "",
		BusinessName: "",
	}

	func() {
		defer func() { _ = recover() }()

		result.Pushname = da.PushName()

		j, ok := da.OwnJID()
		if ok {
			result.JID = string(j)
			if url, id, err := da.ProfilePictureURL(ctx, j); err == nil {
				result.AvatarURL = url
				result.AvatarID = id
			}
			if fullName, bizName, err := da.ContactInfo(ctx, j); err == nil {
				result.FullName = fullName
				result.BusinessName = bizName
			}
		}
	}()

	return result
}

// ErrNoSession é retornado quando não há sessão WhatsApp conectada.
var ErrNoSession = &ProfileError{msg: "no session"}

// ProfileError é um erro tipado para falhas de perfil.
type ProfileError struct {
	msg string
}

// NewProfileError cria um ProfileError com a mensagem informada.
// Útil para testes e para erros com contexto adicional.
func NewProfileError(msg string) *ProfileError {
	return &ProfileError{msg: msg}
}

func (e *ProfileError) Error() string {
	return e.msg
}
