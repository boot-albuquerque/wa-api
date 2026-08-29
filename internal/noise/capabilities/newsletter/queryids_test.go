package newsletter

import (
	"testing"

	"wa-api/internal/noise/protocol/argo"
	"wa-api/internal/noise/protocol/proto/waWa6"
)

// webPayload e noWebInfoPayload eram os dois estados que ConvertQueryID
// distinguia. Continuam aqui porque a distincao volta a importar se o ramo
// desktop for reabilitado (ver o bloco comentado em queryids.go).
func webPayload() *waWa6.ClientPayload {
	return &waWa6.ClientPayload{
		UserAgent: &waWa6.ClientPayload_UserAgent{},
		WebInfo:   &waWa6.ClientPayload_WebInfo{},
	}
}

func noWebInfoPayload() *waWa6.ClientPayload {
	return &waWa6.ClientPayload{UserAgent: &waWa6.ClientPayload_UserAgent{}}
}

// todasAsQueryIDs sao as IDs web que o ramo desktop traduzia.
func todasAsQueryIDs() []string {
	return []string{
		queryFetchNewsletter,
		queryFetchNewsletterDehydrated,
		queryNewslettersDirectory,
		queryRecommendedNewsletters,
		querySubscribedNewsletters,
		queryNewsletterSubscribers,
		mutationMuteNewsletter,
		mutationUnmuteNewsletter,
		mutationUpdateNewsletter,
		mutationCreateNewsletter,
		mutationUnfollowNewsletter,
		mutationFollowNewsletter,
		mutationDemoteAdmin,
		mutationChangeOwner,
		mutationDeleteNewsletter,
	}
}

// Com o ramo desktop desativado, ConvertQueryID e' identidade — para QUALQUER
// payload, inclusive os que antes escolhiam o ramo desktop.
//
// Este teste substitui TestConvertQueryIDDesktopMapeiaTodasAsIDs, que travava a
// tabela de traducao. Se alguem reabilitar o ramo sem atualizar aqui, este
// teste falha e aponta para o bloco comentado em queryids.go.
func TestConvertQueryIDEIdentidadeParaQualquerPayload(t *testing.T) {
	payloads := map[string]*waWa6.ClientPayload{
		"web (com WebInfo)": webPayload(),
		"sem WebInfo":       noWebInfoPayload(),
		"payload vazio":     {},
		"sem UserAgent":     {WebInfo: &waWa6.ClientPayload_WebInfo{}},
	}
	macos := webPayload()
	macos.UserAgent.Platform = waWa6.ClientPayload_UserAgent_MACOS.Enum()
	payloads["plataforma MACOS"] = macos

	for nome, payload := range payloads {
		t.Run(nome, func(t *testing.T) {
			for _, id := range todasAsQueryIDs() {
				if got := ConvertQueryID(payload, id); got != id {
					t.Errorf("ConvertQueryID(%q) = %q, esperava o mesmo ID", id, got)
				}
			}
			// IDs fora da tabela tambem passam intactas.
			if got := ConvertQueryID(payload, "0000000000000000"); got != "0000000000000000" {
				t.Errorf("ID desconhecida = %q, esperava passar intacta", got)
			}
		})
	}
}

// GetUserAgent() devolve nil com o campo ausente. Antes da correcao da F48 o
// codigo fazia `.Platform`, acesso a CAMPO, e isso era SIGSEGV. Hoje a funcao
// nem olha o payload, mas o teste fica: se o ramo desktop voltar, o acesso
// volta com ele.
func TestConvertQueryIDComPayloadVazioNaoPanica(t *testing.T) {
	if got := ConvertQueryID(&waWa6.ClientPayload{}, queryFetchNewsletter); got != queryFetchNewsletter {
		t.Errorf("= %q, esperava %q", got, queryFetchNewsletter)
	}
}

// As duas anomalias da F32 continuam no argo/name-to-queryids.json, e continuam
// travadas aqui — por VALOR LITERAL, ja' que as constantes de desktop estao
// comentadas.
//
// O teste nao serve mais para proteger um caminho vivo (o ramo desktop esta'
// desativado); serve para avisar quem for reabilita-lo de que os dados de
// origem ainda estao errados. Se as anomalias sumirem, ele falha e manda
// atualizar a F32.
func TestAnomaliasDeQueryIDDesktopContinuamNoArgo(t *testing.T) {
	const (
		unfollowDesktop  = "8782612271820087"
		followDesktop    = "8621797084555037"
		subscribedDeskto = "8621797084555037"
	)

	queryIDMap, err := argo.GetQueryIDToMessageName()
	if err != nil {
		t.Fatalf("argo.GetQueryIDToMessageName: %v", err)
	}
	wireStore, err := argo.GetStore()
	if err != nil {
		t.Fatalf("argo.GetStore: %v", err)
	}

	// Anomalia 1: unfollow mapeia para cancelamento de assinatura paga, e esse
	// nome nao existe no wire type store.
	name := queryIDMap[unfollowDesktop]
	if name != "WamoSubCancelSubscription" {
		t.Errorf("unfollow desktop mapeia para %q — atualize a F32 e este teste", name)
	}
	if _, ok := wireStore[name]; ok {
		t.Error("o wire type de WamoSubCancelSubscription apareceu — atualize a F32")
	}

	// Anomalia 2: seguir e "canais assinados" compartilham a mesma ID.
	if followDesktop != subscribedDeskto {
		t.Error("a duplicata conhecida sumiu — atualize a F32 e este teste")
	}

	// E as outras oito continuam com wire type, que e' o que torna as duas
	// acima anomalias e nao a regra.
	for _, id := range []string{
		"9779843322044422", "27256776790637714", "8621797084555037",
		"25403502652570342", "5971669009605755", "6104029483058502",
		"7839742399440946", "27527996220149684",
	} {
		n, ok := queryIDMap[id]
		if !ok {
			t.Errorf("query ID de desktop %q sumiu de name-to-queryids.json", id)
			continue
		}
		if _, ok := wireStore[n]; !ok {
			t.Errorf("query ID %q mapeia para %q, que nao esta' no wire type store", id, n)
		}
	}
}
