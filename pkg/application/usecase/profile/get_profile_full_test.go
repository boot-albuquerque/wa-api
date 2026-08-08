package profile

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// /session/profile/full agrega chamadas de REDE sobre a base local de
// /session/profile. A separação é de custo: a rota barata lê o store e
// responde em microssegundos; esta fala com o servidor do WhatsApp e está
// sujeita a rate limit — medido nesta sessão, um laço de seis consultas de
// perfil disparou `429: rate-overlimit`.

func portasDoFull() (*contractsfake.ProfileAccessProvider, *contractsfake.ContactDirectory, *contractsfake.PrivacyManager) {
	da := &contractsfake.ProfileDataAccess{
		PushNameValue: "Lucas",
		OwnJIDValue:   "5511999@s.whatsapp.net",
		OwnJIDOK:      true,
		DeviceInfoFunc: func() domain.SessionDeviceInfo {
			return domain.SessionDeviceInfo{Platform: "iphone", Connected: true, LoggedIn: true}
		},
	}
	pp := &contractsfake.ProfileAccessProvider{
		ProfileAccessFunc: func(context.Context, string) (port.ProfileDataAccess, error) { return da, nil },
	}
	cd := &contractsfake.ContactDirectory{
		GetUserInfoFunc: func(context.Context, string, []domain.JID) (any, error) {
			return map[string]string{"status": "disponivel"}, nil
		},
	}
	pm := &contractsfake.PrivacyManager{
		GetPrivacySettingsFunc: func(context.Context, string) (any, error) {
			return map[string]string{"last_seen": "contacts"}, nil
		},
	}
	return pp, cd, pm
}

func executarFull(t *testing.T, pp *contractsfake.ProfileAccessProvider, cd *contractsfake.ContactDirectory, pm *contractsfake.PrivacyManager) *ProfileFullResult {
	t.Helper()
	res, err := NewGetProfileFullUseCase(pp, cd, pm, &contractsfake.Logger{}).
		Execute(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("Execute devolveu erro: %v", err)
	}
	return res
}

func TestFull_AgregaBaseLocalERede(t *testing.T) {
	pp, cd, pm := portasDoFull()
	res := executarFull(t, pp, cd, pm)

	if res.Pushname != "Lucas" || res.Platform != "iphone" || !res.Connected {
		t.Errorf("a base local nao veio junto: %+v", res.ProfileResult)
	}
	if res.UserInfo == nil {
		t.Error("user_info ausente no caminho feliz")
	}
	if res.Privacy == nil {
		t.Error("privacy ausente no caminho feliz")
	}
	if res.Unavailable != nil {
		t.Errorf("Unavailable = %+v no caminho feliz, quero nil", res.Unavailable)
	}
}

// TestFull_RedeIndisponivelNaoDerrubaOLocal é a propriedade central: se as
// duas consultas remotas caírem, a resposta ainda traz tudo o que
// /session/profile traria. Devolver erro tornaria a rota indisponível
// justamente quando a rede está ruim — que é quando se quer olhar a sessão.
func TestFull_RedeIndisponivelNaoDerrubaOLocal(t *testing.T) {
	pp, cd, pm := portasDoFull()
	cd.GetUserInfoFunc = func(context.Context, string, []domain.JID) (any, error) {
		return nil, errors.New("usync fora do ar")
	}
	pm.GetPrivacySettingsFunc = func(context.Context, string) (any, error) {
		return nil, errors.New("privacidade fora do ar")
	}

	res := executarFull(t, pp, cd, pm)

	if res.Pushname != "Lucas" || res.Platform != "iphone" {
		t.Errorf("a falha de rede levou junto a base local: %+v", res.ProfileResult)
	}
	for _, chave := range []string{"user_info", "privacy"} {
		motivo, ok := res.Unavailable[chave]
		if !ok || motivo == "" {
			t.Errorf("falha de %q nao foi reportada com motivo: %+v", chave, res.Unavailable)
		}
	}
}

// TestFull_SemJIDProprioNaoConsultaARede: sessão conectada e ainda não
// autenticada não tem JID. Perguntar ao servidor sobre string vazia é
// chamada desperdiçada — e num endpoint com rate limit isso custa.
func TestFull_SemJIDProprioNaoConsultaARede(t *testing.T) {
	pp, cd, pm := portasDoFull()
	da := &contractsfake.ProfileDataAccess{OwnJIDOK: false}
	pp.ProfileAccessFunc = func(context.Context, string) (port.ProfileDataAccess, error) { return da, nil }

	res := executarFull(t, pp, cd, pm)

	if n := len(cd.GetUserInfoCalls); n != 0 {
		t.Errorf("consultou a rede %d vezes sem JID proprio", n)
	}
	if _, ok := res.Unavailable["user_info"]; !ok {
		t.Errorf("a ausencia de JID nao foi declarada: %+v", res.Unavailable)
	}
}

// TestFull_SemSessao_ErroTipado: a fronteira deriva o 400 da categoria, como
// em /session/profile (F83).
func TestFull_SemSessao_ErroTipado(t *testing.T) {
	pp := &contractsfake.ProfileAccessProvider{
		ProfileAccessFunc: func(context.Context, string) (port.ProfileDataAccess, error) {
			return nil, errors.New("sem cliente")
		},
	}

	_, err := NewGetProfileFullUseCase(pp, &contractsfake.ContactDirectory{}, &contractsfake.PrivacyManager{}, &contractsfake.Logger{}).
		Execute(context.Background(), "u-1")

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro sem taxonomia: %T", err)
	}
	if appErr.Category != apperr.CategoryValidation {
		t.Errorf("Category = %q, quero validation", appErr.Category)
	}
}

// TestFull_JSONContinuaPlanoNaBase: o ProfileResult é embutido anônimo, então
// pushname/jid/platform seguem na RAIZ. Perder o anonimato aninharia tudo e
// quebraria quem já consome /session/profile.
func TestFull_JSONContinuaPlanoNaBase(t *testing.T) {
	pp, cd, pm := portasDoFull()
	b, err := json.Marshal(executarFull(t, pp, cd, pm))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var plano map[string]any
	if err := json.Unmarshal(b, &plano); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, chave := range []string{"pushname", "jid", "platform", "connected", "user_info", "privacy"} {
		if _, ok := plano[chave]; !ok {
			t.Errorf("chave %q ausente na raiz: %s", chave, b)
		}
	}
	if _, aninhado := plano["ProfileResult"]; aninhado {
		t.Error("ProfileResult veio aninhado; o embedding perdeu o anonimato")
	}
}
