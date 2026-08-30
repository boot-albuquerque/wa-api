package core

import (
	"context"

	"wa-api/internal/noise/capabilities/user"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waHistorySync"
	"wa-api/internal/noise/protocol/types"
)

// Fachada do dominio de usuario. A logica vive em internal/noise/user e
// opera sobre user.Transport; aqui ficam so' os metodos de *Client que delegam,
// mais os apelidos de tipo que preservam a API historica do pacote.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 7".
//
// Os metodos **exportados** recusam receiver nil com ErrClientIsNil, como as
// fachadas dos lotes 1-6. Os **nao exportados** (usync, getFBIDDevices,
// getFBIDDevicesInternal, parseBlocklist, parseBusinessProfile,
// parseVerifiedNameContent, updatePushName, updateBusinessName,
// handleHistoricalPushNames) NAO ganham essa guarda: eles sao citados por
// internals.go (gerado, fora de escopo — F29) e chamados de dentro do proprio
// cliente, onde `cli` nunca e' nil; adicionar a guarda mudaria assinaturas que
// o gerador copia.
//
// A unica excecao e' `usync`, que JA' tinha a guarda antes desta extracao
// (com teste de regressao proprio) — ela foi preservada literalmente.

// deviceCache e' apelido de tipo, e nao um tipo novo, porque
// notification_device.go monta e le entradas do cache diretamente. Os campos
// eram nao exportados (`devices`, `dhash`) e agora sao `Devices`/`DHash`.
type deviceCache = user.DeviceEntry

// UsyncQueryExtras e' apelido de tipo porque internals.go cita o nome antigo na
// assinatura de DangerousInternalClient.Usync.
type UsyncQueryExtras = user.QueryExtras

// GetProfilePictureParams e' apelido de tipo: e' parte da API publica do
// pacote, usada por chamadores externos em literais compostos com nome de
// campo.
type GetProfilePictureParams = user.GetProfilePictureParams

const (
	BusinessMessageLinkPrefix       = user.BusinessMessageLinkPrefix
	ContactQRLinkPrefix             = user.ContactQRLinkPrefix
	BusinessMessageLinkDirectPrefix = user.BusinessMessageLinkDirectPrefix
	ContactQRLinkDirectPrefix       = user.ContactQRLinkDirectPrefix
)

func (cli *Client) usync(
	ctx context.Context, jids []types.JID, mode, queryContext string,
	query []waBinary.Node, extra ...UsyncQueryExtras,
) (*waBinary.Node, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.USync(ctx, cli.userT(), jids, mode, queryContext, query, extra...)
}

// SetStatusMessage updates the current user's status text, which is shown in the "About" section in the user profile.
//
// This is different from the ephemeral status broadcast messages. Use SendMessage to types.StatusBroadcastJID to send
// such messages.
func (cli *Client) SetStatusMessage(ctx context.Context, msg string) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return user.SetStatusMessage(ctx, cli.userT(), msg)
}

// IsOnWhatsApp checks if the given phone numbers are registered on WhatsApp.
// The phone numbers should be in international format, including the `+` prefix.
func (cli *Client) IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.IsOnWhatsApp(ctx, cli.userT(), phones)
}

// GetUserInfo gets basic user info (avatar, status, verified business name, device list).
func (cli *Client) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.GetInfo(ctx, cli.userT(), jids)
}

func (cli *Client) handleHistoricalPushNames(ctx context.Context, names []*waHistorySync.Pushname) {
	user.HandleHistoricalPushNames(ctx, cli.userT(), names)
}

func (cli *Client) updatePushName(ctx context.Context, jid, jidAlt types.JID, messageInfo *types.MessageInfo, name string) {
	user.UpdatePushName(ctx, cli.userT(), jid, jidAlt, messageInfo, name)
}

func (cli *Client) updateBusinessName(ctx context.Context, jid, jidAlt types.JID, messageInfo *types.MessageInfo, name string) {
	user.UpdateBusinessName(ctx, cli.userT(), jid, jidAlt, messageInfo, name)
}

// GetUserDevicesContext is a deprecated alias of GetUserDevices.
func (cli *Client) GetUserDevicesContext(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	return cli.GetUserDevices(ctx, jids)
}

// GetUserDevices gets the list of devices that the given user has. The input should be a list of
// regular JIDs, and the output will be a list of AD JIDs. The local device will not be included in
// the output even if the user's JID is included in the input. All other devices will be included.
func (cli *Client) GetUserDevices(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.GetDevices(ctx, cli.userT(), jids)
}

// getFBIDDevices consulta dispositivos de JIDs do Messenger.
//
// TOMA o lock do cache, ao contrario de user.GetFBIDDevices, que escreve nele
// sem lock nenhum.
//
// A assimetria e' intencional e fecha a F54. user.GetFBIDDevices nao pode
// travar por conta propria: seu unico chamador de producao, user.GetDevices,
// ja' segura o lock durante toda a chamada, e sync.Mutex nao e' reentrante —
// um Lock() la' seria deadlock imediato. Mas o gerador de internals.go expoe
// TODO metodo nao exportado de *Client, entao esta fachada virava
// DangerousInternalClient.GetFBIDDevices: um caminho publico que escrevia no
// mapa sem sincronizacao nenhuma, o que em Go pode virar
// "fatal error: concurrent map writes" — nao recuperavel.
//
// Travar aqui e' seguro porque esta fachada NAO esta' no caminho de producao:
// user.GetDevices chama user.GetFBIDDevices diretamente. Confirmado por
// `grep -rn "getFBIDDevices\b"`, que devolve so' esta definicao e a chamada
// em internals.go.
func (cli *Client) getFBIDDevices(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	cache := cli.userT().DeviceCache()
	cache.Lock()
	defer cache.Unlock()
	return user.GetFBIDDevices(ctx, cli.userT(), jids)
}

func (cli *Client) getFBIDDevicesInternal(ctx context.Context, jids []types.JID) (*waBinary.Node, error) {
	return user.GetFBIDDevicesInternal(ctx, cli.userT(), jids)
}

// parseFBDeviceList e' usado por notification_device.go, que continua na raiz
// (e' o dominio de notificacao de dispositivo).
func parseFBDeviceList(jid types.JID, deviceList waBinary.Node) deviceCache {
	return user.ParseFBDeviceList(jid, deviceList)
}

// A fachada parseVerifiedNameContent foi REMOVIDA na Fase F/G lote 9. Ela
// existia so' para message_parse.go, que era o unico chamador; agora o dominio
// de mensagem vive em internal/noise/message/ e chama
// user.ParseVerifiedNameContent por import direto, por ser funcao pura. Divida
// do lote 7 fechada.
