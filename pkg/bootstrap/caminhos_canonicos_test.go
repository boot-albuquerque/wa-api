package bootstrap

import (
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// Gate da padronização de caminhos (F269).
//
// O que se afirma: a tabela e as rotas realmente servidas não podem divergir,
// e os caminhos canónicos têm de cumprir a regra que a tabela existe para
// impor. Sem isto, a tabela vira documentação — e documentação de rotas
// desactualiza-se sem partir nada.

// familiasPluralizadas são os primeiros segmentos que a regra obriga a estar
// no plural. Estão aqui, e não deduzidos, porque "colecção ou singleton" é
// juízo semântico: `/session` é singular DE PROPÓSITO, e nenhuma heurística
// sabe disso.
var familiasPluralizadas = map[string]string{
	"chat":       "chats",
	"group":      "groups",
	"user":       "users",
	"newsletter": "newsletters",
	"community":  "communities",
	"message":    "messages",
}

// TestTodaRotaCanonicaEstaRegistadaEALegadaNao é o gate do corte limpo
// (reversão de F269/CAP-10, 2026-08-27): para cada linha da tabela, SÓ a
// forma canónica pode responder — a antiga tem de desaparecer do router, não
// só do contrato documentado. Ver HOUSEKEEP.md para a decisão e a
// justificação (directiva do utilizador, zero consumidores reais antes do
// lançamento).
func TestTodaRotaCanonicaEstaRegistadaEALegadaNao(t *testing.T) {
	servidas := map[string]bool{}
	for _, rota := range Routes(Deps{}) {
		for _, metodo := range rota.Methods {
			servidas[strings.ToUpper(metodo)+" "+rota.Path] = true
		}
	}

	var faltam []string
	for _, linha := range CaminhosCanonicos() {
		antiga := linha.LegacyMethod + " " + linha.LegacyPath
		canonica := linha.CanonicalMethod + " " + linha.CanonicalPath
		if servidas[antiga] {
			faltam = append(faltam, "a rota legada "+antiga+" AINDA responde — devia ter sido substituída pela canónica")
		}
		if !servidas[canonica] {
			faltam = append(faltam, canonica+" está na tabela mas NÃO foi registada")
		}
	}
	sort.Strings(faltam)
	if len(faltam) > 0 {
		t.Errorf("%d divergências entre a tabela e as rotas servidas:\n  %s",
			len(faltam), strings.Join(faltam, "\n  "))
	}
}

// TestRotaLegadaResponde404 é o teste de regressão exigido pela política
// anti-regressão do projeto para achados corrigidos (CLAUDE.md): reproduz,
// para as noventa e uma rotas da tabela, exactamente a condição que a
// reversão de F269/CAP-10 muda — o caminho antigo deixa de casar com
// qualquer rota registada. Table-driven sobre caminhos.tsv, e não uma lista à
// mão, porque são noventa e uma entradas e a tabela já é a fonte única.
func TestRotaLegadaResponde404(t *testing.T) {
	router := newRouterForRouteCheck()

	for _, linha := range CaminhosCanonicos() {
		linha := linha
		t.Run(linha.LegacyMethod+"_"+linha.LegacyPath, func(t *testing.T) {
			req := httptest.NewRequest(linha.LegacyMethod, linha.LegacyPath, nil)
			var match mux.RouteMatch
			if router.Match(req, &match) {
				t.Fatalf("%s %s ainda casa com uma rota registada — devia ter sido removida no corte limpo (F269/CAP-10 revertido)",
					linha.LegacyMethod, linha.LegacyPath)
			}
		})
	}
}

// consolidadasCAP10 são rotas legadas cuja forma canónica não é uma
// renomeação 1-para-1 (o que CaminhosCanonicos() assume), mas uma
// consolidação de VÁRIAS rotas legadas para UMA canónica nova com
// manipulador diferente do delas — ver api/openapi/CAMINHOS-CANONICOS.md,
// secção CAP-10, e a mesma exceção em openapi_coverage_test.go.
var consolidadasCAP10 = map[string]bool{
	"POST /chat/downloadimage":    true,
	"POST /chat/downloadvideo":    true,
	"POST /chat/downloadaudio":    true,
	"POST /chat/downloaddocument": true,
	"POST /chat/downloadsticker":  true,
}

// TestNenhumaFamiliaDeColeccaoFicouNoSingular é o teste que impede a
// padronização de ficar a meio: se alguém acrescentar `/group/coisa-nova` sem
// linha na tabela, isto acusa.
func TestNenhumaFamiliaDeColeccaoFicouNoSingular(t *testing.T) {
	comCanonica := map[string]bool{}
	for _, linha := range CaminhosCanonicos() {
		comCanonica[linha.LegacyMethod+" "+linha.LegacyPath] = true
	}

	var semPadronizar []string
	for _, rota := range Routes(Deps{}) {
		primeiro := strings.Split(strings.TrimPrefix(rota.Path, "/"), "/")[0]
		if _, deviaSerPlural := familiasPluralizadas[primeiro]; !deviaSerPlural {
			continue
		}
		for _, metodo := range rota.Methods {
			chave := strings.ToUpper(metodo) + " " + rota.Path
			if !comCanonica[chave] && !consolidadasCAP10[chave] {
				semPadronizar = append(semPadronizar, chave)
			}
		}
	}
	sort.Strings(semPadronizar)
	if len(semPadronizar) > 0 {
		t.Errorf("%d rotas de família de colecção sem forma canónica — "+
			"acrescente-as a api/openapi/caminhos.tsv (e a pkg/bootstrap/caminhos.tsv):\n  %s",
			len(semPadronizar), strings.Join(semPadronizar, "\n  "))
	}
}

// TestCaminhosCanonicosCumpremARegra afirma a regra sobre a própria tabela:
// um canónico no singular, ou com a relação colada, seria a padronização a
// documentar-se a si mesma errada.
func TestCaminhosCanonicosCumpremARegra(t *testing.T) {
	// relacoesColadas são as palavras que a regra 2 proíbe num segmento: elas
	// significam que o caminho está a transportar a relação no NOME em vez de
	// na estrutura.
	relacoesColadas := []string{"requestparticipants", "updateparticipants",
		"joinapprovalmode", "updaterequestparticipants"}

	var mal []string
	for _, linha := range CaminhosCanonicos() {
		primeiro := strings.Split(strings.TrimPrefix(linha.CanonicalPath, "/"), "/")[0]
		for singular, plural := range familiasPluralizadas {
			if primeiro == singular {
				mal = append(mal, linha.CanonicalPath+": primeiro segmento no singular, devia ser /"+plural)
			}
		}
		for _, colada := range relacoesColadas {
			if strings.Contains(linha.CanonicalPath, colada) {
				mal = append(mal, linha.CanonicalPath+": transporta a relação no nome ("+colada+")")
			}
		}
	}
	sort.Strings(mal)
	if len(mal) > 0 {
		t.Errorf("%d caminhos canónicos que não cumprem a própria regra:\n  %s",
			len(mal), strings.Join(mal, "\n  "))
	}
}

// TestAsDuasTabelasDeCaminhosSaoIguais: existe uma cópia em api/openapi (que é
// onde se edita e onde o gerador de OpenAPI lê) e outra em pkg/bootstrap (que
// o go:embed exige, por não poder sair do próprio directório).
//
// Duas cópias divergem. Este teste é o que as mantém iguais.
func TestAsDuasTabelasDeCaminhosSaoIguais(t *testing.T) {
	fonte, err := lerFicheiro("../../api/openapi/caminhos.tsv")
	if err != nil {
		t.Fatalf("ler a tabela de origem: %v", err)
	}
	embutida, err := lerFicheiro("caminhos.tsv")
	if err != nil {
		t.Fatalf("ler a tabela embutida: %v", err)
	}
	if fonte != embutida {
		t.Error("api/openapi/caminhos.tsv e pkg/bootstrap/caminhos.tsv divergem — " +
			"copie a primeira para a segunda; a de api/openapi é a que se edita")
	}
}

func lerFicheiro(caminho string) (string, error) {
	bruto, err := os.ReadFile(caminho)
	if err != nil {
		return "", err
	}
	return string(bruto), nil
}
