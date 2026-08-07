// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package send reune o caminho de SAIDA de mensagens do fork: o fluxo waE2E
// (SendMessage) e o fluxo v3/FB (SendFBMessage).
//
// Os dois protocolos moram no MESMO pacote de proposito. Eles nao sao
// independentes: compartilham as constantes de no e de atributo do <message>,
// participantListHashV2, copyAttrs, MessageDebugTimings, SendResponse,
// SendRequestExtra e o par awaitSendAck/applySendAck. Separar em `send` e
// `sendfb` obrigaria um dos dois a importar o outro (nao seria mais estreito,
// so' teria uma fronteira a mais) ou a duplicar o substrato compartilhado — e
// duplicar constantes de wire e a funcao de phash e' exatamente o tipo de
// divergencia silenciosa que a Fase F/G existe para evitar.
package send

import (
	"context"
	"sync"
	"time"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// Errors reune os sentinelas de erro do pacote RAIZ de que este dominio
// precisa. Hoje e' um so'.
//
// ErrNotLoggedIn nao migrou para send/errors.go, ao contrario dos outros seis
// sentinelas deste caminho, porque nao e' deste dominio: 15 arquivos da raiz o
// devolvem (presence, call, msgsecret, mediaretry, ...). Reatribui-lo a partir
// daqui faria o dominio de envio "dono" de um erro que e' do fork inteiro. Ele
// atravessa a interface pelo mesmo racional de group.IQErrors (lote 6) e
// user.IQErrors (lote 7): e' o MESMO ponteiro da raiz, nao uma copia, entao
// errors.Is continua casando exatamente como antes da extracao.
type Errors struct {
	NotLoggedIn error
}

// Transport e' a fatia do cliente de que o dominio de envio precisa.
//
// E' o transporte mais largo da Fase F/G, mais largo ate' que o de retry, e o
// motivo e' estrutural: enviar uma mensagem e' o caminho que TOCA TODOS OS
// OUTROS DOMINIOS. Resolver o destino consulta grupo e usuario; cifrar consulta
// prekeys e a sessao Signal; o <message> carrega reporting token, tctoken e
// cstoken; o ack invalida os caches de grupo e de dispositivo. Cada metodo
// abaixo corresponde a uma chamada que o codigo fazia em *Client antes da
// extracao — nenhum foi inventado para conveniencia.
//
// # Fachada da raiz, e nao import direto de group/user/prekeys/tctoken
//
// Este pacote depende de quatro dominios ja' extraidos (lotes 4 a 7). A escolha
// deliberada foi alcanca-los pelas FACHADAS da raiz (os metodos abaixo), e nao
// importando group/, user/, prekeys/ e tctoken/ e chamando suas free functions
// direto. Nao ha ciclo em nenhuma das duas opcoes — os quatro sao folhas —,
// entao a decisao e' de desenho:
//
//   - As free functions daqueles pacotes recebem os transportes DELES
//     (group.Transport, user.Transport, prekeys.Transport, ...). Chama-las
//     daqui obrigaria send.Transport a expor quatro interfaces inteiras de
//     outros dominios so' para alcancar 2-4 funcoes de cada uma. O adaptador da
//     raiz ja' constroi esses transportes; passar por ele custa um metodo por
//     chamada, contra ~40 metodos de interface transitivos.
//   - Os dublês de teste deste pacote precisariam implementar as quatro
//     interfaces alheias, e um teste de envio nao tem por que saber montar um
//     <iq> de usync.
//   - As fachadas da raiz (cli.getCachedGroupData, cli.GetUserDevices,
//     cli.fetchPreKeysNoError, cli.ensureTCToken) sao exatamente o que o codigo
//     original chamava. Passar por elas mantem a extracao literal: nenhum
//     caminho de chamada novo foi criado.
//
// As DUAS excecoes, ambas por serem dado e nao comportamento:
//
//   - `group` e' importado aqui para os tipos *group.Meta (retorno de
//     CachedGroupData) e group.ErrNotFound (o valor por tras de
//     whatsmeow.ErrGroupNotFound). Traduzir o Meta para uma struct local seria
//     uma copia campo a campo sem ganho.
//   - `tctoken.ShouldSendInChatAction` e `tctoken.ShouldSendNew` sao chamadas
//     direto em outbound.go. Sao funcoes PURAS (JID -> bool, time -> bool), sem
//     transporte nem estado; as proprias fachadas da raiz sao uma linha de
//     delegacao. Faze-las atravessar a interface seria transformar duas funcoes
//     puras em dois metodos de duble.
//
// Deliberadamente nao expoe nada do *whatsmeow.Client alem disso: e' o que
// permite que este pacote nao importe o pacote raiz (o que fecharia um ciclo) e
// que os testes usem um duble em vez de um cliente com socket e sessao Noise.
type Transport interface {
	// --- sessao ---

	// Store e' o device store da sessao. Aparece inteiro pelo mesmo racional
	// dos lotes 3 a 7: `store` ja' e' subpacote folha do fork, expo-lo nao cria
	// dependencia nova nem ciclo. Aqui ele e' usado de quatro formas: como
	// store Signal (session.NewBuilderFromSignal, groups.NewGroupSessionBuilder),
	// como cache de sessoes (WithCachedSessions/PutCachedSessions), como mapa
	// LID/PN (Store.LIDs) e como origem do device identity (Store.Account).
	Store() *store.Device
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// OwnID e' o JID (PN ou LID, conforme o login) deste dispositivo.
	OwnID() types.JID
	// OwnLID e' o LID deste dispositivo.
	OwnLID() types.JID
	// GenerateMessageID gera o ID quando o chamador nao fornece um.
	GenerateMessageID() types.MessageID
	// Errors devolve os sentinelas da raiz. Ver Errors.
	Errors() Errors

	// --- configuracao do cliente ---

	// IsMessenger espelha `cli.MessengerConfig != nil`. As duas leituras do
	// campo neste caminho sao a mesma pergunta ("estamos na ponte do
	// Messenger?"), e as duas decidem a mesma coisa: se o <device-identity>
	// acompanha um pkmsg.
	IsMessenger() bool
	// AutoTrustIdentity espelha Client.AutoTrustIdentity.
	AutoTrustIdentity() bool
	// DefaultRequestTimeout entrega o defaultRequestTimeout da raiz. E'
	// constante do substrato de requisicao (request.go), nao deste dominio,
	// entao continua definida la'.
	DefaultRequestTimeout() time.Duration

	// --- serializacao e transporte do stanza ---

	// SendLock e' o mutex que serializa TODOS os envios do cliente. O ponteiro
	// precisa ser estavel: e' campo de *Client e um sync.Mutex nunca pode ser
	// copiado por valor.
	//
	// A secao critica NAO mudou de forma nesta extracao — continua sendo
	// "trava antes de gravar a mensagem recente, libera no fim da funcao de
	// envio", cobrindo cifragem e escrita no socket. Ver PATCHES.md, lote 8.
	SendLock() *sync.Mutex
	// WaitResponse registra a espera pela resposta de um ID de requisicao.
	WaitResponse(reqID string) chan *waBinary.Node
	// CancelResponse desfaz o registro feito por WaitResponse.
	CancelResponse(reqID string, ch chan *waBinary.Node)
	// SendNodeAndGetData envia um no e devolve os bytes do frame enviado (que
	// o caminho de retry precisa para reenviar o mesmo frame).
	SendNodeAndGetData(ctx context.Context, node waBinary.Node) ([]byte, error)
	// IsDisconnectNode diz se a "resposta" foi na verdade um <stream:error> de
	// desconexao. Vive em request.go, na raiz, com o resto do substrato.
	IsDisconnectNode(node *waBinary.Node) bool
	// RetryFrame reenvia o frame apos uma reconexao.
	RetryFrame(
		ctx context.Context,
		reqType, id string,
		data []byte,
		origResp *waBinary.Node,
		timeout time.Duration,
	) (*waBinary.Node, error)
	// AddRecentMessage guarda a mensagem no cache de reenvio. Exatamente um
	// dos dois payloads e' nao-nil: `wa` no caminho waE2E, `fb` no v3.
	AddRecentMessage(
		ctx context.Context,
		to types.JID,
		id types.MessageID,
		wa *waE2E.Message,
		fb *waMsgApplication.MessageApplication,
	) error

	// --- dominios ja' extraidos, alcancados pela fachada da raiz ---

	// CachedGroupData devolve os metadados do grupo (lote 6). Pode devolver
	// (nil, nil) quando o servidor ecoa um `id` diferente do consultado — os
	// dois chamadores tratam isso explicitamente.
	CachedGroupData(ctx context.Context, jid types.JID) (*group.Meta, error)
	// BroadcastListParticipants devolve os membros de uma lista de transmissao.
	BroadcastListParticipants(ctx context.Context, jid types.JID) ([]types.JID, error)
	// UserDevices expande JIDs de usuario em JIDs de dispositivo (lote 7).
	UserDevices(ctx context.Context, jids []types.JID) ([]types.JID, error)
	// UserInfo consulta o usync (lote 7). Este caminho so' le o campo LID, mas
	// a fachada da raiz e' a exportada historicamente e devolve o mapa inteiro.
	UserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error)
	// FetchPreKeysNoError busca bundles de prekey (lote 4), engolindo o erro —
	// o nome preserva o contrato do fetchPreKeysNoError original.
	FetchPreKeysNoError(ctx context.Context, devices []types.JID) map[types.JID]*prekey.Bundle
	// InvalidateGroupCache derruba a entrada de grupo do cache (lote 6).
	InvalidateGroupCache(jid types.JID)
	// InvalidateDeviceCache derruba a lista de dispositivos do cache (lote 7).
	//
	// InvalidateGroupCache/InvalidateDeviceCache existem em vez de expor
	// *group.Cache e *user.DeviceCache porque este dominio so' faz `delete`
	// nos dois — e Delete e' justamente o unico metodo dos dois caches que
	// sincroniza sozinho, exatamente porque o chamador (applySendAck) tomava
	// o lock so' para isso. Ver group/cache.go e user/cache.go.
	InvalidateDeviceCache(jid types.JID)

	// --- sessao Signal ---

	// MigrateSessionStore move a sessao Signal de um JID de telefone para o LID.
	MigrateSessionStore(ctx context.Context, pn, lid types.JID)
	// ClearUntrustedIdentity apaga a identidade guardada de um alvo, para que o
	// bundle de prekey seja reprocessado (so' com AutoTrustIdentity).
	ClearUntrustedIdentity(ctx context.Context, target types.JID) error

	// --- tokens que acompanham o stanza ---

	// ShouldIncludeReportingToken espelha cli.shouldIncludeReportingToken.
	ShouldIncludeReportingToken(message *waE2E.Message) bool
	// MessageReportingToken monta o no <reporting_token>. A logica depende de
	// generateMsgSecretKey e da tabela de campos de reportingtoken.go, que nao
	// foi extraida — so' a construcao atravessa a interface.
	MessageReportingToken(
		msgProtobuf []byte,
		msg *waE2E.Message,
		senderJID, remoteJID types.JID,
		messageID types.MessageID,
	) waBinary.Node
	// ApplyBotMessageHKDF deriva o BotMessageSecret do segredo da mensagem. A
	// derivacao vive em msgsecret_keys.go, na raiz, junto do resto das chaves
	// de segredo de mensagem — dominio que nao foi extraido neste lote.
	ApplyBotMessageHKDF(messageSecret []byte) []byte
	// EnsureTCToken devolve o privacy token guardado e nao expirado (lote 4).
	EnsureTCToken(ctx context.Context, jid types.JID) ([]byte, error)
	// ResolveTCTokenStorageLID resolve sob qual JID o tctoken e' guardado.
	ResolveTCTokenStorageLID(ctx context.Context, jid types.JID) types.JID
	// TCTokenSenderTS le o timestamp de emissao guardado em memoria.
	TCTokenSenderTS(jid types.JID) time.Time
	// IssuePrivacyTokenAndSave emite um privacy token novo e o guarda.
	IssuePrivacyTokenAndSave(jid types.JID, senderTimestamp time.Time)
	// GenerateCsToken gera o <cstoken>, alternativa ao <tctoken>.
	GenerateCsToken(ctx context.Context, jid types.JID) []byte
}
