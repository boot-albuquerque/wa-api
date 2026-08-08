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

// O perfil da sessão devolvia só seis campos de identidade pública
// (pushname, avatar, jid, nomes). Identidade e estado do aparelho — LID,
// plataforma, se está conectado, se está autenticado — já estavam no store
// local, sem custo de rede, e não saíam em lugar nenhum.
//
// A distinção connected/logged_in é a que mais importa na prática: são
// estados diferentes, e foi exatamente a confusão entre eles que a F80
// tornou visível.

func devInfoCheio() domain.SessionDeviceInfo {
	return domain.SessionDeviceInfo{
		LID:                   "90937376170214:17@lid",
		Platform:              "iphone",
		RegistrationID:        123456,
		LIDMigrationTimestamp: 1754600000,
		Initialized:           true,
		Connected:             true,
		LoggedIn:              true,
	}
}

func TestBuildProfile_IncluiDadosDoAparelho(t *testing.T) {
	da := &contractsfake.ProfileDataAccess{
		DeviceInfoFunc: devInfoCheio,
	}

	got := buildProfile(context.Background(), da, &contractsfake.Logger{})

	if da.DeviceInfoCalls != 1 {
		t.Fatalf("DeviceInfo chamado %d vezes, quero 1", da.DeviceInfoCalls)
	}
	if got.SessionDeviceInfo != devInfoCheio() {
		t.Errorf("dados do aparelho nao chegaram ao resultado: %+v", got.SessionDeviceInfo)
	}
}

// TestBuildProfile_AparelhoSemJID: DeviceInfo é lido ANTES de OwnJID de
// propósito. Uma sessão conectada mas ainda não autenticada não tem JID, e
// é justamente nela que saber `connected=true, logged_in=false` tem valor.
// Se alguém mover a chamada para dentro do `if ok`, este teste acusa.
func TestBuildProfile_AparelhoSemJID(t *testing.T) {
	da := &contractsfake.ProfileDataAccess{
		OwnJIDOK: false, // sessao conectada e ainda nao autenticada
		DeviceInfoFunc: func() domain.SessionDeviceInfo {
			return domain.SessionDeviceInfo{Platform: "iphone", Connected: true}
		},
	}

	got := buildProfile(context.Background(), da, &contractsfake.Logger{})

	if got.JID != "" {
		t.Fatalf("JID = %q, quero vazio", got.JID)
	}
	if !got.Connected || got.Platform != "iphone" {
		t.Errorf("dados do aparelho sumiram numa sessao sem JID: %+v", got.SessionDeviceInfo)
	}
}

// TestProfileResult_ContinuaObjetoPlano: os campos novos entram embutidos,
// e não aninhados. Um cliente que já lia `pushname` na raiz não pode passar
// a precisar descer um nível — e o embedding só produz JSON plano se o
// campo permanecer anônimo.
func TestProfileResult_ContinuaObjetoPlano(t *testing.T) {
	b, err := json.Marshal(ProfileResult{
		Pushname:          "Alice",
		SessionDeviceInfo: devInfoCheio(),
	})
	if err != nil {
		t.Fatalf("marshal falhou: %v", err)
	}

	var plano map[string]any
	if err := json.Unmarshal(b, &plano); err != nil {
		t.Fatalf("unmarshal falhou: %v", err)
	}
	for _, chave := range []string{
		"pushname", "avatar_url", "jid", "full_name", "business_name",
		"lid", "platform", "registration_id", "initialized",
		"connected", "logged_in", "lid_migration_timestamp",
	} {
		if _, ok := plano[chave]; !ok {
			t.Errorf("chave %q ausente na raiz do JSON: %s", chave, b)
		}
	}
	if _, aninhado := plano["SessionDeviceInfo"]; aninhado {
		t.Error("os campos do aparelho vieram aninhados; o embedding perdeu o anonimato")
	}
}

// TestExecute_SemSessao_ErroTipado fecha a corrente da F83.
//
// O teste de handler (profile_envelope_test.go) constrói o apperr ele mesmo,
// então prova que o handler REAGE à taxonomia — não que este use case a
// PRODUZ. Sem esta asserção, remover o apperr daqui deixaria o handler
// correto e a rota errada, com os dois lados verdes.
func TestExecute_SemSessao_ErroTipado(t *testing.T) {
	uc := NewGetProfileUseCase(
		&contractsfake.ProfileAccessProvider{
			ProfileAccessFunc: func(context.Context, string) (port.ProfileDataAccess, error) {
				return nil, errors.New("sem cliente")
			},
		},
		&contractsfake.Logger{},
	)

	_, err := uc.Execute(context.Background(), "u-1")

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro nao carrega taxonomia: %T %v — a fronteira nao consegue derivar o status", err, err)
	}
	if appErr.Category != apperr.CategoryValidation {
		t.Errorf("Category = %q, quero %q — sem sessao e' recusa de cliente, nao falha do servidor",
			appErr.Category, apperr.CategoryValidation)
	}
	if appErr.Code != "no_session" {
		t.Errorf("Code = %q, quero no_session", appErr.Code)
	}
	// A causa continua sendo ErrNoSession: quem já dependia de errors.Is não
	// pode ter sido quebrado pela mudança.
	if !errors.Is(err, ErrNoSession) {
		t.Error("errors.Is(err, ErrNoSession) deixou de valer")
	}
}
