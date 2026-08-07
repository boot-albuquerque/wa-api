package newsletter

import "wa-api/internal/wa-noise/protocol/proto/waWa6"

const (
	queryFetchNewsletter           = "6563316087068696"
	queryFetchNewsletterDehydrated = "7272540469429201"
	queryRecommendedNewsletters    = "7263823273662354" // variables -> input -> {limit: 20, country_codes: [string]}, output: xwa2_newsletters_recommended
	queryNewslettersDirectory      = "6190824427689257" // variables -> input -> {view: "RECOMMENDED", limit: 50, start_cursor: base64, filters: {country_codes: [string]}}
	querySubscribedNewsletters     = "6388546374527196" // variables -> empty, output: xwa2_newsletter_subscribed
	queryNewsletterSubscribers     = "9800646650009898" // variables -> input -> {newsletter_id, count}, output: xwa2_newsletter_subscribers -> subscribers -> edges
	mutationMuteNewsletter         = "6274038279359549" // variables -> {newsletter_id, updates->{description, settings}}, output: xwa2_newsletter_update -> NewsletterMetadata without viewer meta
	mutationUnmuteNewsletter       = "6068417879924485"
	mutationUpdateNewsletter       = "7150902998257522"
	mutationCreateNewsletter       = "6234210096708695"
	mutationUnfollowNewsletter     = "6392786840836363"
	mutationFollowNewsletter       = "9926858900719341"
)

// ---------------------------------------------------------------------------
// RAMO DESKTOP DESATIVADO (F31 e F32 em HOUSEKEEP.md)
// ---------------------------------------------------------------------------
//
// O bloco abaixo traduzia cada query ID web para a equivalente de desktop. Foi
// desativado em 2026-08-07, nao apagado, para que os valores continuem visiveis
// para quem precisar reabilita-lo.
//
// POR QUE FOI DESATIVADO
//
//  1. Ele nunca dispara com o nosso cliente. A condicao era
//     `plataforma == MACOS || GetWebInfo() == nil`, e as duas metades sao falsas
//     aqui: a comparacao de plataforma e' entre dois PONTEIROS diferentes, logo
//     sempre falsa (F31), e store.BaseClientPayload sempre preenche WebInfo —
//     getRegistrationPayload e getLoginPayload clonam dele, e nada zera o campo.
//  2. Duas das dez constantes estao ERRADAS (F32), e estao anotadas abaixo.
//  3. O motivo original de existirem — alimentar o wire type Argo, que so' e'
//     indexado por ID de desktop em argo/name-to-queryids.json — nao vale mais:
//     decodeArgoResult comeca com `if true { return ErrArgoDecodingBroken }`,
//     ou seja, o caminho Argo inteiro ja' esta' desligado no fork.
//
// O QUE PRECISA ACONTECER PARA REABILITAR
//
//   - substituir as duas constantes erradas por valores capturados de um
//     cliente desktop real (o bundle JS do WhatsApp Web carrega as query IDs
//     como constantes; e' a rota usual de extracao);
//   - decidir se a comparacao de plataforma deve passar a ser por VALOR
//     (`GetPlatform() == waWa6.ClientPayload_UserAgent_MACOS`), que e' o que
//     consertaria a F31 — e que so' faz sentido depois de (1), porque hoje
//     ligaria um caminho com IDs comprovadamente quebradas;
//   - reabilitar decodeArgoResult, sem o que a resposta nao tem como ser
//     decodificada.
//
// Para referencia: o Baileys, a outra implementacao aberta do protocolo, NAO
// tem separacao desktop/web nenhuma — usa um unico enum QueryIds, sem
// ramificacao por plataforma.
//
//	const (
//		queryFetchNewsletterDesktop        = "9779843322044422"
//		queryRecommendedNewslettersDesktop = "27256776790637714"
//		querySubscribedNewslettersDesktop  = "8621797084555037"
//		queryNewsletterSubscribersDesktop  = "25403502652570342"
//		mutationMuteNewsletterDesktop      = "5971669009605755"
//		mutationUnmuteNewsletterDesktop    = "6104029483058502"
//		mutationUpdateNewsletterDesktop    = "7839742399440946"
//		mutationCreateNewsletterDesktop    = "27527996220149684"
//
//		// ERRADA (F32): mapeia, em argo/name-to-queryids.json, para
//		// "WamoSubCancelSubscription" — cancelamento de assinatura PAGA, nao
//		// "deixar de seguir canal" — e esse nome nao existe no wire type store.
//		// E' a unica das dez sem wire type.
//		mutationUnfollowNewsletterDesktop = "8782612271820087"
//
//		// ERRADA (F32): valor IDENTICO a querySubscribedNewslettersDesktop.
//		// Em cliente desktop, "seguir canal" dispararia a consulta de canais
//		// assinados e a operacao nao faria nada, em silencio.
//		mutationFollowNewsletterDesktop = "8621797084555037"
//	)
//
//	switch queryID {
//	case queryFetchNewsletter:
//		return queryFetchNewsletterDesktop
//	case queryRecommendedNewsletters:
//		return queryRecommendedNewslettersDesktop
//	case querySubscribedNewsletters:
//		return querySubscribedNewslettersDesktop
//	case queryNewsletterSubscribers:
//		return queryNewsletterSubscribersDesktop
//	case mutationMuteNewsletter:
//		return mutationMuteNewsletterDesktop
//	case mutationUnmuteNewsletter:
//		return mutationUnmuteNewsletterDesktop
//	case mutationUpdateNewsletter:
//		return mutationUpdateNewsletterDesktop
//	case mutationCreateNewsletter:
//		return mutationCreateNewsletterDesktop
//	case mutationUnfollowNewsletter:
//		return mutationUnfollowNewsletterDesktop
//	case mutationFollowNewsletter:
//		return mutationFollowNewsletterDesktop
//	}

// ConvertQueryID devolve a query ID inalterada.
//
// A funcao continua existindo, com o payload no lugar, porque SendMexIQ a chama
// e porque e' aqui que a traducao para desktop voltaria caso seja reabilitada —
// ver o bloco comentado acima.
func ConvertQueryID(_ *waWa6.ClientPayload, queryID string) string {
	return queryID
}
