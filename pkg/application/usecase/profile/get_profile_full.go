package profile

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// GetProfileFullUseCase entrega o perfil da sessão AGREGANDO consultas de
// rede, e por isso vive separado de GetProfileUseCase.
//
// A separação é de custo, não de organização: `/session/profile` lê só o
// store local e responde em microssegundos; este aqui fala com o servidor do
// WhatsApp e está sujeito a latência e a rate limit — medido nesta sessão,
// um laço de seis consultas de perfil disparou `429: rate-overlimit`. Juntar
// os dois tornaria a rota barata refém da cara.
type GetProfileFullUseCase struct {
	profiles appport.ProfileAccessProvider
	contacts appport.ContactRoster
	privacy  appport.PrivacyManager
	logger   appport.Logger
}

// NewGetProfileFullUseCase cria o use case com as portas injetadas.
func NewGetProfileFullUseCase(
	pp appport.ProfileAccessProvider,
	cd appport.ContactRoster,
	pm appport.PrivacyManager,
	logger appport.Logger,
) *GetProfileFullUseCase {
	return &GetProfileFullUseCase{profiles: pp, contacts: cd, privacy: pm, logger: logger}
}

// ProfileFullResult é o perfil da sessão com o que só a rede sabe.
type ProfileFullResult struct {
	// ProfileResult embutido: quem já consome `/session/profile` encontra
	// exatamente os mesmos campos, na raiz, e não precisa aprender uma
	// segunda forma para os dados que não mudaram.
	ProfileResult

	// UserInfo são os metadados do PRÓPRIO JID — o recado ("about"/status) e
	// a lista de dispositivos que o servidor conhece. Era `any` com o tipo do
	// SDK dentro; a porta passou a ser tipada na migração da família de
	// utilizadores, e o adaptador é que normaliza.
	UserInfo []domain.UserInfo `json:"user_info,omitempty"`

	// Privacy são as configurações de privacidade da conta.
	Privacy domain.PrivacySettings `json:"privacy,omitempty"`

	// Unavailable diz o que NÃO pôde ser obtido, com o motivo. O perfil
	// degrada em vez de falhar — mas degradar em silêncio é pior que
	// falhar: o cliente veria campos ausentes sem saber se é ausência de
	// dado ou falha de rede.
	Unavailable map[string]string `json:"unavailable,omitempty"`
}

// Execute monta o perfil completo.
//
// A base local NUNCA falha por causa da rede: se as duas consultas remotas
// caírem, a resposta ainda traz tudo o que `/session/profile` traria. O
// contrário — devolver erro porque a privacidade não veio — tornaria a rota
// indisponível justamente quando a rede está ruim, que é quando se quer
// olhar o estado da sessão.
func (uc *GetProfileFullUseCase) Execute(ctx context.Context, txtID string) (*ProfileFullResult, error) {
	da, err := uc.profiles.ProfileAccess(ctx, txtID)
	if err != nil {
		uc.logger.Warn(ctx, "no session for txtID", "txtID", txtID, "error", err)
		return nil, apperr.New("no_session", apperr.CategoryValidation, "no session", false, ErrNoSession)
	}

	res := &ProfileFullResult{
		ProfileResult: buildProfile(ctx, da, uc.logger),
		Unavailable:   map[string]string{},
	}

	uc.preencherUserInfo(ctx, txtID, res)
	uc.preencherPrivacidade(ctx, txtID, res)

	if len(res.Unavailable) == 0 {
		res.Unavailable = nil
	}
	return res, nil
}

// preencherUserInfo consulta o servidor sobre o PRÓPRIO JID.
//
// Sem JID próprio não há o que perguntar — é o estado de uma sessão
// conectada e ainda não autenticada, e ali a ausência é resposta, não falha.
func (uc *GetProfileFullUseCase) preencherUserInfo(ctx context.Context, txtID string, res *ProfileFullResult) {
	if res.JID == "" {
		res.Unavailable["user_info"] = "sessao sem JID proprio; nada a consultar"
		return
	}
	info, err := uc.contacts.GetUserInfo(ctx, txtID, []domain.JID{domain.JID(res.JID)})
	if err != nil {
		uc.logger.Warn(ctx, "dados de usuario indisponiveis", "error", err, "txtID", txtID)
		res.Unavailable["user_info"] = err.Error()
		return
	}
	res.UserInfo = info
}

func (uc *GetProfileFullUseCase) preencherPrivacidade(ctx context.Context, txtID string, res *ProfileFullResult) {
	p, err := uc.privacy.GetPrivacySettings(ctx, txtID)
	if err != nil {
		uc.logger.Warn(ctx, "configuracoes de privacidade indisponiveis", "error", err, "txtID", txtID)
		res.Unavailable["privacy"] = err.Error()
		return
	}
	res.Privacy = p
}
