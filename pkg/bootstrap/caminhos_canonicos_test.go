package bootstrap

import (
	"net/http"
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
//
// As cinco rotas de download (/chat/download{tipo}) não estão em
// CaminhosCanonicos() — sofreram a mesma reversão por um caminho separado
// (worktree http-dto-download-paths, F328), sem passar por caminhos.tsv, e
// já têm o seu próprio teste de regressão (TestF297_LegacyDownloadRoutesAreGone).
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
			if !comCanonica[chave] {
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

// TestF297_LegacyDownloadRoutesAreGone é o controlo negativo da reversão da
// política CAP-10/F269 para download (HOUSEKEEP.md F297): as cinco rotas
// legadas por-kind não são apenas "não documentadas" — deixaram de existir
// no router de produção. Prova pela ROTA REGISTRADA de verdade
// (`Routes(Deps{})`, a mesma tabela que wiring_routes.go produz), não por
// suposição sobre o código.
func TestF297_LegacyDownloadRoutesAreGone(t *testing.T) {
	legacy := []string{
		"/chat/downloadimage",
		"/chat/downloadvideo",
		"/chat/downloadaudio",
		"/chat/downloaddocument",
		"/chat/downloadsticker",
	}

	servidas := map[string]bool{}
	for _, rota := range Routes(Deps{}) {
		servidas[rota.Path] = true
	}
	for _, path := range legacy {
		if servidas[path] {
			t.Errorf("%s: ainda está em Routes(Deps{}) — F297 devia tê-la removido", path)
		}
	}
	if !servidas["/chats/download/{kind}"] {
		t.Fatal("/chats/download/{kind} não está em Routes(Deps{}) — a rota consolidada tem de sobreviver à reversão")
	}

	// E pela resposta HTTP real: sem NENHUMA rota casando, o mux devolve 404
	// — não 405, porque o caminho em si não existe mais, nenhum outro método
	// disputa o mesmo padrão.
	router := newRouterForRouteCheck()
	for _, path := range legacy {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: got %d, want 404 (rota removida pela F297)", path, rec.Code)
			}
		})
	}

	// A rota consolidada continua casada para os cinco kinds — o
	// comportamento completo (200, MIME, bytes) já é coberto por
	// TestDownload_Success_ViaRegisteredRoute em
	// pkg/presentation/http/handlers/handler_download_test.go; aqui só se
	// confirma que o ROTEADOR DE PRODUÇÃO ainda casa o padrão.
	for _, kind := range []string{"image", "video", "audio", "document", "sticker"} {
		t.Run("chats/download/"+kind, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/chats/download/"+kind, nil)
			var match mux.RouteMatch
			if !router.Match(req, &match) {
				t.Fatalf("POST /chats/download/%s não casou: %v", kind, match.MatchErr)
			}
		})
	}
}
