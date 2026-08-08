package user_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// GetUserProfileUseCase aceita QUALQUER uma das identidades de um contato —
// telefone, JID de telefone ou LID — e resolve a contraparte. PN e LID são o
// mesmo tipo Go, distintos só pelo sufixo em tempo de execução (F65), então a
// direção da resolução é decisão de runtime e precisa ser testada nos dois
// sentidos.

const perfilUser = "user-1"

// portaDePerfil monta um ContactDirectory com os dois sentidos de resolução
// pré-carregados, e um JIDResolver que qualifica número nu.
func portaDePerfil() (*contractsfake.ContactDirectory, *contractsfake.JIDResolver) {
	cd := &contractsfake.ContactDirectory{
		GetLIDForPNFunc: func(_ context.Context, _ string, jid domain.JID) (domain.JID, error) {
			if jid == "5511999@s.whatsapp.net" {
				return "90937@lid", nil
			}
			return "", nil
		},
		GetPNForLIDFunc: func(_ context.Context, _ string, lid domain.JID) (domain.JID, error) {
			if lid == "90937@lid" {
				return "5511999@s.whatsapp.net", nil
			}
			return "", nil
		},
		IsOnWhatsAppFunc: func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
			return []domain.WhatsAppCheck{{IsIn: true, VerifiedName: "Loja Teste"}}, nil
		},
		GetProfilePictureFunc: func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
			return &domain.AvatarInfo{URL: "https://exemplo/foto.jpg", ID: "pic-1"}, nil
		},
		GetUserInfoFunc: func(context.Context, string, []domain.JID) (any, error) {
			return map[string]string{"status": "disponivel"}, nil
		},
	}
	// O dublê espelha a regra REAL de mapping/jid/parse.go: sem "@", aplica o
	// servidor padrão; com "@", preserva o que veio. A primeira versão deste
	// fake qualificava tudo, era mais permissiva que a produção, e por isso
	// deixou passar um bug em que telefone nu dava erro na rota de verdade.
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if !strings.Contains(raw, "@") {
				return domain.JID(raw + "@s.whatsapp.net"), nil
			}
			return domain.JID(raw), nil
		},
	}
	return cd, jr
}

func executar(t *testing.T, cd *contractsfake.ContactDirectory, jr *contractsfake.JIDResolver, alvo string) *user.UserProfileResult {
	t.Helper()
	res, err := user.NewGetUserProfileUseCase(cd, jr, &contractsfake.Logger{}).
		Execute(context.Background(), perfilUser, alvo)
	if err != nil {
		t.Fatalf("Execute(%q) devolveu erro: %v", alvo, err)
	}
	return res
}

// TestPerfil_AceitaAsTresFormas é o coração do endpoint: telefone nu, JID de
// telefone e LID têm de produzir O MESMO par de identidades. Se um dos três
// caminhos parar de resolver, o cliente que usa aquele formato recebe meia
// resposta sem erro nenhum.
func TestPerfil_AceitaAsTresFormas(t *testing.T) {
	for _, alvo := range []string{"5511999", "5511999@s.whatsapp.net", "90937@lid"} {
		t.Run(alvo, func(t *testing.T) {
			cd, jr := portaDePerfil()

			res := executar(t, cd, jr, alvo)

			if res.JID != "5511999@s.whatsapp.net" {
				t.Errorf("JID = %q, quero 5511999@s.whatsapp.net", res.JID)
			}
			if res.LID != "90937@lid" {
				t.Errorf("LID = %q, quero 90937@lid", res.LID)
			}
			if res.Query != alvo {
				t.Errorf("Query = %q, quero %q — o eco do pedido se perdeu", res.Query, alvo)
			}
		})
	}
}

// TestPerfil_DirecaoDaResolucao trava a direção: LID de entrada NÃO pode
// chamar GetLIDForPN, e telefone de entrada não pode chamar GetPNForLID.
// Sem isto, inverter as duas chamadas ainda passaria no teste acima quando os
// dublês fossem simétricos — foi exatamente assim que a F65 sobreviveu.
func TestPerfil_DirecaoDaResolucao(t *testing.T) {
	t.Run("telefone consulta LID", func(t *testing.T) {
		cd, jr := portaDePerfil()

		executar(t, cd, jr, "5511999@s.whatsapp.net")

		if len(cd.GetLIDForPNCalls) != 1 {
			t.Errorf("GetLIDForPN chamado %d vezes, quero 1", len(cd.GetLIDForPNCalls))
		}
		if n := len(cd.GetPNForLIDCalls); n != 0 {
			t.Errorf("GetPNForLID chamado %d vezes para um telefone", n)
		}
	})

	t.Run("LID consulta telefone", func(t *testing.T) {
		cd, jr := portaDePerfil()

		executar(t, cd, jr, "90937@lid")

		if len(cd.GetPNForLIDCalls) != 1 {
			t.Errorf("GetPNForLID chamado %d vezes, quero 1", len(cd.GetPNForLIDCalls))
		}
		if n := len(cd.GetLIDForPNCalls); n != 0 {
			t.Errorf("GetLIDForPN chamado %d vezes para um LID", n)
		}
	})
}

// TestPerfil_NumeroSemWhatsApp: ausência de conta é RESPOSTA, não erro — a
// decisão de contrato é 200 com on_whatsapp:false. Um 404 obrigaria o cliente
// a separar "não existe" de "falhou" pelo corpo.
func TestPerfil_NumeroSemWhatsApp(t *testing.T) {
	cd, jr := portaDePerfil()
	cd.IsOnWhatsAppFunc = func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
		return []domain.WhatsAppCheck{{IsIn: false}}, nil
	}

	res := executar(t, cd, jr, "5511999")

	if res.OnWhatsApp == nil {
		t.Fatal("OnWhatsApp nulo; quero false explicito — nulo significa 'nao perguntei'")
	}
	if *res.OnWhatsApp {
		t.Error("OnWhatsApp = true para numero sem conta")
	}
}

// TestPerfil_FalhaParcialDegrada: uma foto indisponível não pode custar o
// resto do perfil. E o motivo tem de aparecer — degradar em silêncio faria o
// cliente ver campo vazio sem saber se é ausência de dado ou falha de rede.
func TestPerfil_FalhaParcialDegrada(t *testing.T) {
	cd, jr := portaDePerfil()
	cd.GetProfilePictureFunc = func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
		return nil, errors.New("rede fora do ar")
	}

	res := executar(t, cd, jr, "5511999")

	if res.JID == "" || res.LID == "" {
		t.Errorf("a falha do avatar levou junto as identidades: %+v", res)
	}
	if res.AvatarURL != "" {
		t.Errorf("AvatarURL = %q apos falha, quero vazio", res.AvatarURL)
	}
	motivo, ok := res.Unavailable["avatar"]
	if !ok {
		t.Fatalf("a falha do avatar nao foi reportada: %+v", res.Unavailable)
	}
	if motivo == "" {
		t.Error("motivo vazio; o cliente nao consegue distinguir ausencia de falha")
	}
}

// TestPerfil_SemMapeamentoNaoEhFalha: um telefone sem LID conhecido é comum e
// não é erro. O campo fica vazio E o motivo é declarado.
func TestPerfil_SemMapeamentoNaoEhFalha(t *testing.T) {
	cd, jr := portaDePerfil()
	cd.GetLIDForPNFunc = func(context.Context, string, domain.JID) (domain.JID, error) {
		return "", nil
	}

	res := executar(t, cd, jr, "5511999")

	if res.LID != "" {
		t.Errorf("LID = %q sem mapeamento, quero vazio", res.LID)
	}
	if _, ok := res.Unavailable["lid"]; !ok {
		t.Errorf("ausencia de mapeamento nao foi declarada: %+v", res.Unavailable)
	}
}

// TestPerfil_TudoOK_SemBlocoUnavailable: quando nada falha, o campo some do
// JSON (omitempty sobre mapa nil). Um `unavailable: {}` vazio em toda resposta
// bem-sucedida treinaria o cliente a ignorá-lo.
func TestPerfil_TudoOK_SemBlocoUnavailable(t *testing.T) {
	cd, jr := portaDePerfil()

	res := executar(t, cd, jr, "5511999")

	if res.Unavailable != nil {
		t.Errorf("Unavailable = %+v no caminho feliz, quero nil", res.Unavailable)
	}
	if res.VerifiedName != "Loja Teste" {
		t.Errorf("VerifiedName = %q, quero 'Loja Teste'", res.VerifiedName)
	}
	if res.AvatarURL == "" || res.UserInfo == nil {
		t.Errorf("perfil incompleto no caminho feliz: %+v", res)
	}
}

// TestPerfil_SemSessao_Recusa: sem sessão não há o que consultar, e o erro da
// porta tem de chegar intacto à fronteira, que decide o status por ele.
func TestPerfil_SemSessao_Recusa(t *testing.T) {
	semSessao := errors.New("porta: sem sessao")
	cd, jr := portaDePerfil()
	cd.SessionGuard = contractsfake.FailSession(semSessao)

	_, err := user.NewGetUserProfileUseCase(cd, jr, &contractsfake.Logger{}).
		Execute(context.Background(), perfilUser, "5511999")

	if !errors.Is(err, semSessao) {
		t.Fatalf("a causa da porta se perdeu: %v", err)
	}
	if n := len(cd.GetLIDForPNCalls); n != 0 {
		t.Errorf("consultou a porta %d vezes sem sessao", n)
	}
}
