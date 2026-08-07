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

	// desktop & mobile
	queryFetchNewsletterDesktop        = "9779843322044422"
	queryRecommendedNewslettersDesktop = "27256776790637714"
	querySubscribedNewslettersDesktop  = "8621797084555037"
	queryNewsletterSubscribersDesktop  = "25403502652570342"
	mutationMuteNewsletterDesktop      = "5971669009605755" // variables -> {newsletter_id, updates->{description, settings}}, output: xwa2_newsletter_update -> NewsletterMetadata without viewer meta
	mutationUnmuteNewsletterDesktop    = "6104029483058502"
	mutationUpdateNewsletterDesktop    = "7839742399440946"
	mutationCreateNewsletterDesktop    = "27527996220149684"
	mutationUnfollowNewsletterDesktop  = "8782612271820087"
	mutationFollowNewsletterDesktop    = "8621797084555037"
)

// ConvertQueryID traduz uma query ID web para a equivalente de desktop quando o
// payload indica um cliente desktop/companion. IDs fora da tabela passam
// intactas.
//
// Recebe o payload em vez de um Transport de proposito: a decisao nao depende
// de mais nada do cliente, e assim a funcao e' testavel sem duble.
//
// Cuidado ao mexer: a comparacao de plataforma abaixo compara dois PONTEIROS
// diferentes e por isso e' sempre falsa — na pratica so' o GetWebInfo() == nil
// decide. E' bug herdado do upstream, registrado em HOUSEKEEP.md (F31) e
// preservado aqui de proposito.
func ConvertQueryID(payload *waWa6.ClientPayload, queryID string) string {
	// GetUserAgent() devolve nil com o campo ausente, e `.Platform` e' acesso a
	// CAMPO — dava SIGSEGV em vez de ler o zero (F48). GetPlatform() e' o getter
	// gerado, que trata o receptor nil.
	//
	// A comparacao continua entre PONTEIROS e continua sempre falsa (F31), de
	// proposito: comparar por VALOR faria clientes MacOS passarem a usar as
	// query IDs de desktop, e a F32 documenta que duas delas estao ERRADAS no
	// upstream. Consertar a F31 aqui ligaria um caminho comprovadamente
	// quebrado — as duas so' podem ser resolvidas juntas, com IDs corretas em
	// maos.
	if payload.GetUserAgent().GetPlatform().Enum() == waWa6.ClientPayload_UserAgent_MACOS.Enum() || payload.GetWebInfo() == nil {
		switch queryID {
		case queryFetchNewsletter:
			return queryFetchNewsletterDesktop
		case queryRecommendedNewsletters:
			return queryRecommendedNewslettersDesktop
		case querySubscribedNewsletters:
			return querySubscribedNewslettersDesktop
		case queryNewsletterSubscribers:
			return queryNewsletterSubscribersDesktop
		case mutationMuteNewsletter:
			return mutationMuteNewsletterDesktop
		case mutationUnmuteNewsletter:
			return mutationUnmuteNewsletterDesktop
		case mutationUpdateNewsletter:
			return mutationUpdateNewsletterDesktop
		case mutationCreateNewsletter:
			return mutationCreateNewsletterDesktop
		case mutationUnfollowNewsletter:
			return mutationUnfollowNewsletterDesktop
		case mutationFollowNewsletter:
			return mutationFollowNewsletterDesktop
		default:
			return queryID
		}
	} else {
		return queryID
	}
}
