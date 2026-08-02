package profile

import (
	"context"
	"encoding/json"

	appport "wa-api/pkg/application/contracts"
)

// GetProfileUseCase implementa a lógica de obtenção de perfil WhatsApp.
type GetProfileUseCase struct {
	profiles appport.ProfileAccessProvider
	logger   appport.Logger
}

// NewGetProfileUseCase cria o usecase com dependências injetadas.
func NewGetProfileUseCase(
	profiles appport.ProfileAccessProvider,
	logger appport.Logger,
) *GetProfileUseCase {
	return &GetProfileUseCase{
		profiles: profiles,
		logger:   logger,
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
	da, err := uc.profiles.ProfileAccess(ctx, txtID)
	if err != nil {
		uc.logger.Warn(ctx, "no session for txtID", "txtID", txtID, "error", err)
		return "", ErrNoSession
	}

	result := buildProfile(ctx, da, uc.logger)

	// ProfileResult é composto exclusivamente de campos string, e json.Marshal
	// não tem como falhar para esse conjunto de tipos — UTF-8 inválido é
	// substituído, não rejeitado. O tratamento de erro que existia aqui era
	// inalcançável: nenhuma execução o atingia e nenhum teste podia cobri-lo.
	// Se ProfileResult ganhar um campo que não seja string (map, chan, func,
	// interface), reintroduza a verificação de erro junto com o campo.
	responseJSON, _ := json.Marshal(result)
	return string(responseJSON), nil
}

// buildProfile constrói o perfil a partir do ProfileDataAccess.
// Extraída para teste unitário sem depender de *whatsmeow.Client.
// Chama OwnJID() uma única vez e cacheia o resultado para reuso.
func buildProfile(ctx context.Context, da appport.ProfileDataAccess, logger appport.Logger) ProfileResult {
	result := ProfileResult{
		Pushname:     "",
		AvatarURL:    "",
		AvatarID:     "",
		JID:          "",
		FullName:     "",
		BusinessName: "",
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(ctx, "recovered from panic while building profile", "panic", r)
			}
		}()

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
