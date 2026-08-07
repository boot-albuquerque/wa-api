package newsletter

import (
	"testing"

	"wa-api/internal/wa-noise/protocol/argo"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
)

// webPayload e desktopPayload sao os dois estados que ConvertQueryID distingue.
//
// UserAgent precisa estar presente nos dois: ConvertQueryID faz
// `payload.GetUserAgent().Platform`, acesso a CAMPO (nao ao getter), que estoura
// nil deref se UserAgent for nil. Em producao o payload vem de
// store.Device.GetClientPayload(), que sempre preenche UserAgent, entao o caso
// nao acontece — mas e' fragilidade herdada do upstream, registrada em
// HOUSEKEEP.md (F48).
func webPayload() *waWa6.ClientPayload {
	return &waWa6.ClientPayload{
		UserAgent: &waWa6.ClientPayload_UserAgent{},
		WebInfo:   &waWa6.ClientPayload_WebInfo{},
	}
}

func desktopPayload() *waWa6.ClientPayload {
	return &waWa6.ClientPayload{UserAgent: &waWa6.ClientPayload_UserAgent{}}
}

// Com WebInfo presente (cliente web), as query IDs passam intactas.
func TestConvertQueryIDWebMantemIDs(t *testing.T) {
	for _, id := range []string{
		queryFetchNewsletter,
		queryRecommendedNewsletters,
		querySubscribedNewsletters,
		queryNewsletterSubscribers,
		mutationMuteNewsletter,
		mutationUnmuteNewsletter,
		mutationUpdateNewsletter,
		mutationCreateNewsletter,
		mutationUnfollowNewsletter,
		mutationFollowNewsletter,
	} {
		if got := ConvertQueryID(webPayload(), id); got != id {
			t.Errorf("ConvertQueryID(%q) = %q, esperava o mesmo ID", id, got)
		}
	}
}

// Sem WebInfo (desktop/companion), cada ID web vira o ID desktop equivalente.
// Um mapeamento errado aqui faz o servidor recusar a consulta inteira.
func TestConvertQueryIDDesktopMapeiaTodasAsIDs(t *testing.T) {
	for web, desktop := range map[string]string{
		queryFetchNewsletter:        queryFetchNewsletterDesktop,
		queryRecommendedNewsletters: queryRecommendedNewslettersDesktop,
		querySubscribedNewsletters:  querySubscribedNewslettersDesktop,
		queryNewsletterSubscribers:  queryNewsletterSubscribersDesktop,
		mutationMuteNewsletter:      mutationMuteNewsletterDesktop,
		mutationUnmuteNewsletter:    mutationUnmuteNewsletterDesktop,
		mutationUpdateNewsletter:    mutationUpdateNewsletterDesktop,
		mutationCreateNewsletter:    mutationCreateNewsletterDesktop,
		mutationUnfollowNewsletter:  mutationUnfollowNewsletterDesktop,
		mutationFollowNewsletter:    mutationFollowNewsletterDesktop,
	} {
		if got := ConvertQueryID(desktopPayload(), web); got != desktop {
			t.Errorf("ConvertQueryID(%q) = %q, esperava %q", web, got, desktop)
		}
	}
}

// IDs fora da tabela (e as proprias IDs de desktop) passam sem traducao.
func TestConvertQueryIDDesktopPassaDesconhecidas(t *testing.T) {
	for _, id := range []string{
		queryFetchNewsletterDehydrated,
		queryNewslettersDirectory,
		queryFetchNewsletterDesktop,
		"0000000000000000",
	} {
		if got := ConvertQueryID(desktopPayload(), id); got != id {
			t.Errorf("ConvertQueryID(%q) = %q, esperava o mesmo ID", id, got)
		}
	}
}

// Documenta um bug herdado do upstream: ConvertQueryID compara
// `payload.GetUserAgent().Platform == waWa6...MACOS.Enum()`, ou seja, dois
// PONTEIROS diferentes — a comparacao e' sempre falsa. Na pratica so' o
// `GetWebInfo() == nil` decide. Ver HOUSEKEEP.md (F31).
func TestConvertQueryIDPlatformMacOSNaoDecideSozinho(t *testing.T) {
	payload := webPayload()
	payload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_MACOS.Enum()
	// WebInfo continua presente, entao o ramo desktop nao e' escolhido, apesar
	// da plataforma MACOS.
	if got := ConvertQueryID(payload, queryFetchNewsletter); got != queryFetchNewsletter {
		t.Errorf("ConvertQueryID = %q, esperava %q (a comparacao de ponteiro e' inerte)", got, queryFetchNewsletter)
	}
}

// Toda ID de desktop emitida por ConvertQueryID precisa ter um wire type Argo
// correspondente; sem isso a decodificacao Argo nao teria como montar a
// resposta quando o caminho for reabilitado.
func TestQueryIDsDesktopTemWireTypeArgo(t *testing.T) {
	queryIDMap, err := argo.GetQueryIDToMessageName()
	if err != nil {
		t.Fatalf("argo.GetQueryIDToMessageName: %v", err)
	}
	wireStore, err := argo.GetStore()
	if err != nil {
		t.Fatalf("argo.GetStore: %v", err)
	}

	for _, id := range []string{
		queryFetchNewsletterDesktop,
		queryRecommendedNewslettersDesktop,
		querySubscribedNewslettersDesktop,
		queryNewsletterSubscribersDesktop,
		mutationMuteNewsletterDesktop,
		mutationUnmuteNewsletterDesktop,
		mutationUpdateNewsletterDesktop,
		mutationCreateNewsletterDesktop,
	} {
		name, ok := queryIDMap[id]
		if !ok {
			t.Errorf("query ID de desktop %q nao esta' em name-to-queryids.json", id)
			continue
		}
		if _, ok := wireStore[name]; !ok {
			t.Errorf("query ID %q mapeia para %q, que nao esta' no wire type store", id, name)
		}
	}
}

// mutationUnfollowNewsletterDesktop e' a unica ID de desktop que nao tem wire
// type Argo: ela mapeia para "WamoSubCancelSubscription" (cancelamento de
// assinatura paga, nao "deixar de seguir canal") e esse nome nao existe no
// wire type store. Registrado em HOUSEKEEP.md (F32); o teste trava o estado
// atual para que a correcao seja deliberada.
func TestQueryIDUnfollowDesktopSemWireTypeArgo(t *testing.T) {
	queryIDMap, err := argo.GetQueryIDToMessageName()
	if err != nil {
		t.Fatalf("argo.GetQueryIDToMessageName: %v", err)
	}
	wireStore, err := argo.GetStore()
	if err != nil {
		t.Fatalf("argo.GetStore: %v", err)
	}
	name := queryIDMap[mutationUnfollowNewsletterDesktop]
	if name != "WamoSubCancelSubscription" {
		t.Fatalf("mapeamento mudou para %q — atualize HOUSEKEEP.md (F32) e este teste", name)
	}
	if _, ok := wireStore[name]; ok {
		t.Fatal("o wire type apareceu — atualize HOUSEKEEP.md (F32) e este teste")
	}
}

// mutationFollowNewsletterDesktop e querySubscribedNewslettersDesktop tem o
// MESMO valor no upstream, e por isso "seguir canal" no desktop dispara a
// consulta de canais assinados. Registrado em HOUSEKEEP.md (F32); o teste
// trava o estado atual para que a correcao seja deliberada.
func TestQueryIDsDesktopDuplicadaConhecida(t *testing.T) {
	if mutationFollowNewsletterDesktop != querySubscribedNewslettersDesktop {
		t.Fatal("a duplicata conhecida sumiu — atualize HOUSEKEEP.md (F32) e este teste")
	}
}

// GetUserAgent() devolve nil com o campo ausente, e `.Platform` era acesso a
// campo — SIGSEGV em vez de zero (F48). Em producao o payload vem de
// store.Device.GetClientPayload(), que sempre preenche UserAgent, entao era
// fragilidade latente; este teste garante que continue latente.
func TestConvertQueryIDComPayloadVazioNaoPanica(t *testing.T) {
	got := ConvertQueryID(&waWa6.ClientPayload{}, queryFetchNewsletter)
	// Sem WebInfo o ramo de desktop e' escolhido pelo segundo operando.
	if got != queryFetchNewsletterDesktop {
		t.Errorf("= %q, esperava %q", got, queryFetchNewsletterDesktop)
	}
}
