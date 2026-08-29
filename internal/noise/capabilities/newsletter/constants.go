// Package newsletter implementa o dominio de canais (newsletters) do WhatsApp:
// metadados, listagem paginada de mensagens e updates, recibos de visualizacao,
// reacoes, criacao e as mutations de seguir/silenciar.
//
// Todo o pacote opera sobre a interface Transport (ver transport.go) em vez de
// sobre *noise.Client. E' o que permite que ele nao importe o pacote raiz —
// o inverso fecharia ciclo, porque a raiz precisa chamar de volta o dominio.
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 2".
package newsletter

// LinkPrefix e' o prefixo dos links de convite de canal. O pacote raiz reexporta
// este valor como NewsletterLinkPrefix.
const LinkPrefix = "https://whatsapp.com/channel/"

// Nomes do wire format compartilhados pelo caminho de newsletter. So' entram
// aqui os que aparecem em mais de um lugar; os nomes de tag e atributo usados
// uma unica vez, no ponto onde o no e' montado ou lido, continuam literais —
// ver PATCHES.md (Fase E, lote 2).
//
// As query IDs do MEX ficam em queryids.go, junto do mapeamento web -> desktop
// que e' o unico consumidor delas.
const (
	// Namespace e' o namespace <iq> das consultas de newsletter que nao
	// passam pelo MEX (live updates, listagem de mensagens e updates).
	Namespace = "newsletter"

	// liveUpdatesTag e' o no de assinatura temporaria de updates ao vivo: vai
	// na requisicao e e' relido na resposta para extrair a duracao.
	liveUpdatesTag = "live_updates"
	// liveUpdatesDurationAttr carrega, em segundos, por quanto tempo a
	// assinatura vale.
	liveUpdatesDurationAttr = "duration"

	// messagesTag e' o no de resposta paginada de mensagens. GetMessageUpdates
	// tambem usa esta tag desde 2026-08-28 (LIB-03) — o `<message_updates>`
	// separado foi abandonado, o servidor parou de responder a ele.
	messagesTag = "messages"
	// messagesErrContext e' o campo In dos ElementMissingError dos dois
	// getters paginados.
	messagesErrContext = "newsletter messages response"
)

const (
	// mexNamespace e' o namespace <iq> das consultas GraphQL/MEX do WhatsApp.
	mexNamespace = "w:mex"
	// mexQueryTag / mexQueryIDAttr / mexResultTag nomeiam o no de requisicao e
	// o de resposta do MEX, que aparecem tanto na construcao quanto na leitura.
	mexQueryTag    = "query"
	mexQueryIDAttr = "query_id"
	mexResultTag   = "result"
	// mexFormatAttr / mexFormatArgo identificam a resposta codificada em Argo
	// (em vez de JSON puro).
	mexFormatAttr = "format"
	mexFormatArgo = "argo"
	// mexErrContext e' o campo In do ElementMissingError da resposta MEX.
	mexErrContext = "mex response"
)
