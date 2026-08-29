package bootstrap

import (
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"

	"wa-api/internal/noise/protocol/types/events"
)

// Deduplicação de mensagem reentregue (F103).
//
// # O defeito
//
// Quando o WhatsApp reentrega uma mensagem, processávamos a segunda cópia como
// se fosse nova: baixávamos a mídia de novo e despachávamos o webhook de novo.
// Para o cliente, a mesma mensagem chega duas vezes com o MESMO `messageID` — e
// quem usa webhook para criar pedido, atendimento ou cobrança duplica, sem que
// nada na nossa documentação avise que ele precisa deduplicar.
//
// # Por que a primeira cópia é guardada, e não a última
//
// A investigação da F103 mostrou que a PRIMEIRA cópia é a metadata-incompleta:
// ela vem do reenvio pedido ao telefone, que o SDK monta com `ParseWebMessage`
// (`core/client_session.go:75-84`) — sem `Type`, sem `MediaType`, e com
// `PushName` possivelmente vazio. A cópia AO VIVO, com metadados completos,
// chegou ~4 minutos DEPOIS.
//
// Isso me fez recomendar "manter a última". A recomendação não sobrevive à
// linha do tempo: quando a segunda cópia chega, a primeira já foi entregue há
// minutos. "Manter a última" exigiria DESENTREGAR, que não existe.
//
// Então a escolha real é entre dois danos:
//
//   - entregar duas vezes — o cliente duplica pedido ou cobrança;
//   - entregar uma vez com metadado pobre — falta `pushName` e `type`.
//
// O primeiro é pior, e por uma margem grande: duplicar dinheiro é incidente,
// metadado ausente é degradação. Guardamos a primeira e suprimimos a segunda.
//
// # Mas o custo fica MEDIDO, não escondido
//
// Ao suprimir, comparamos o que a cópia descartada trazia. Se ela tinha
// metadado que a entregue não tinha, isso vai para o log com os campos
// nomeados. Sem essa linha, a decisão acima seria uma aposta permanente; com
// ela, dá para saber com que frequência o metadado pobre realmente escapa e
// rever a política com dado em vez de instinto.
const (
	// dedupTTL é por quanto tempo um messageID é lembrado.
	//
	// O intervalo MEDIDO entre as duas cópias foi de ~4 minutos. Dez dá margem
	// de 2,5x sobre o único caso observado. Trinta segundos — que é o número
	// que se escolhe por instinto para "deduplicar" — não teria pegado nada.
	dedupTTL = 10 * time.Minute

	// dedupCleanup é o intervalo do coletor do cache. Mais folgado que o TTL
	// porque a expiração já é verificada na leitura; o coletor só devolve
	// memória.
	dedupCleanup = 15 * time.Minute
)

// mensagemVista é o que se lembra de uma mensagem já processada.
//
// Só metadado, nunca o payload: guardar conteúdo aqui multiplicaria por dez
// minutos a superfície de qualquer vazamento de memória ou de dump.
type mensagemVista struct {
	Tipo        string
	TipoDeMidia string
	PushName    string
	// TemConteudoUtilizavel é F358, não F103: F103 media só metadado
	// (Type/MediaType/PushName) — os dois exemplos investigados TINHAM o
	// payload, só faltava o rótulo. F358 é o caso que F103 não previu: o
	// `type=media` chega correto, mas o Message decodificado não tem
	// NENHUM dos campos de mídia (Image/Audio/Document/Video/Sticker/
	// Album) — é um envelope SenderKeyDistributionMessage, a primeira
	// metade de uma entrega de grupo/broadcast, sem o conteúdo em si.
	// Medido em `POST /status/set/video`: a PRIMEIRA cópia chega assim,
	// tipada como mídia mas vazia; a mídia de verdade chega segundos
	// depois, com o MESMO message_id — e a F103 original suprimia essa
	// segunda cópia porque Type/MediaType/PushName são idênticos nas
	// duas, então o teste de "perdeu" (linhas 129-138) nunca disparava.
	TemConteudoUtilizavel bool
}

// temConteudoDeMidiaUtilizavel diz se este evento, sendo do tipo "media",
// carrega pelo menos um dos payloads que processMessageMedia sabe baixar.
//
// Mensagens que não são de mídia (texto, etc.) não têm este modo de falha —
// contam sempre como "com conteúdo utilizável", para que a comparação em
// mensagemJaProcessada nunca as trate como uma entrega incompleta.
//
// log:exempt predicado puro sem I/O nem erro; o único chamador
// (mensagemJaProcessada) já loga o resultado desta checagem quando ele muda
// o desfecho (achado F358) — logar aqui também duplicaria a mesma decisão.
func temConteudoDeMidiaUtilizavel(info *events.Message) bool {
	if info.Info.Type != messageWireTypeMedia {
		return true
	}
	msg := info.Message
	return msg.GetImageMessage() != nil ||
		msg.GetAudioMessage() != nil ||
		msg.GetDocumentMessage() != nil ||
		msg.GetVideoMessage() != nil ||
		msg.GetStickerMessage() != nil ||
		msg.GetAlbumMessage() != nil
}

// mensagensVistas é o cache de deduplicação.
//
// Sem teto explícito de entradas, e isso é deliberado: o limite aqui é
// TEMPORAL. Cada entrada são três strings curtas e some em dez minutos, então
// o pior caso é o volume de dez minutos de mensagens — diferente da F86, onde
// o item retido era trabalho em voo com payload junto.
var mensagensVistas = cache.New(dedupTTL, dedupCleanup)

// chaveDedup separa usuários.
//
// O `messageID` do WhatsApp é único por remetente, não globalmente. Duas
// sessões nossas podem legitimamente ver o mesmo id vindo de conversas
// diferentes, e uma chave só pelo id faria a mensagem de um usuário suprimir a
// de outro — que seria uma perda silenciosa muito pior que a duplicata.
func chaveDedup(userID, messageID string) string {
	return userID + "\x00" + messageID
}

// mensagemJaProcessada informa se esta mensagem já passou por aqui, e registra
// o que a cópia descartada trazia de metadado a mais.
//
// Devolve false — deixando passar — quando o `messageID` vem vazio. Sem id não
// há como deduplicar, e suprimir por chave vazia colapsaria mensagens sem
// relação nenhuma numa só.
func mensagemJaProcessada(userID string, info *events.Message) bool {
	if info == nil || info.Info.ID == "" {
		return false
	}

	chave := chaveDedup(userID, info.Info.ID)
	atual := mensagemVista{
		Tipo:                  info.Info.Type,
		TipoDeMidia:           info.Info.MediaType,
		PushName:              info.Info.PushName,
		TemConteudoUtilizavel: temConteudoDeMidiaUtilizavel(info),
	}

	anterior, existe := mensagensVistas.Get(chave)
	if !existe {
		mensagensVistas.Set(chave, atual, cache.DefaultExpiration)
		return false
	}

	primeira, ok := anterior.(mensagemVista)
	if !ok {
		// Tipo inesperado no cache: trata como não-visto em vez de descartar a
		// mensagem. O erro de deixar passar uma duplicata é recuperável; o de
		// engolir uma mensagem legítima não é.
		mensagensVistas.Set(chave, atual, cache.DefaultExpiration)
		return false
	}

	// F358: a primeira cópia dizia "media" mas não tinha payload nenhum — é
	// o envelope de estabelecimento de sessão de grupo, não o conteúdo. Isto
	// NÃO é o caso que F103 escolheu suprimir (duas cópias com o MESMO
	// conteúdo, uma com metadado pior); é a mídia em si chegando pela
	// primeira vez, com o mesmo message_id de uma tentativa anterior que
	// nunca a carregou. Deixar passar não duplica nada — a cópia suprimida
	// nunca tinha o que baixar nem o que entregar.
	if !primeira.TemConteudoUtilizavel && atual.TemConteudoUtilizavel {
		log.Info().
			Str("userid", userID).
			Str("message_id", info.Info.ID).
			Msg("mensagem reentregue NAO suprimida; a primeira copia nao tinha midia utilizavel (F358)")
		mensagensVistas.Set(chave, atual, cache.DefaultExpiration)
		return false
	}

	// Nomear o metadado que a cópia suprimida trazia e a entregue não é o que
	// transforma a escolha de política em algo medível. Inline, e não em função
	// própria, porque separada ela é função elegível sem nada para logar (sem
	// erro, sem I/O) — mesma razão de `publishCapabilities` não ter um
	// `buildCapabilityReport` ao lado.
	var perdeu []string
	if primeira.Tipo == "" && atual.Tipo != "" {
		perdeu = append(perdeu, "type")
	}
	if primeira.TipoDeMidia == "" && atual.TipoDeMidia != "" {
		perdeu = append(perdeu, "media_type")
	}
	if primeira.PushName == "" && atual.PushName != "" {
		perdeu = append(perdeu, "pushname")
	}
	if len(perdeu) > 0 {
		log.Warn().
			Str("userid", userID).
			Str("message_id", info.Info.ID).
			Strs("metadado_perdido", perdeu).
			Msg("mensagem reentregue suprimida; a copia descartada trazia metadado que a entregue nao tinha (F103)")
	} else {
		log.Info().
			Str("userid", userID).
			Str("message_id", info.Info.ID).
			Msg("mensagem reentregue suprimida; nenhuma entrega duplicada foi feita")
	}

	return true
}
