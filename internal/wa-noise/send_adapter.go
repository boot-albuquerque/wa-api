// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"sync"
	"time"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waMsgApplication"
	"wa-api/internal/wa-noise/send"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// sendTransport adapta *Client a send.Transport. Existe para que o pacote
// internal/wa-noise/send possa operar sobre uma interface estreita sem importar
// o pacote raiz (o que fecharia um ciclo) e sem que *Client precise ganhar
// metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 8".
type sendTransport struct {
	cli *Client
}

var _ send.Transport = sendTransport{}

// sendT devolve o adaptador de envio deste cliente.
func (cli *Client) sendT() send.Transport {
	return sendTransport{cli}
}

func (t sendTransport) Store() *store.Device { return t.cli.Store }

func (t sendTransport) Log() waLog.Logger { return t.cli.Log }

func (t sendTransport) OwnID() types.JID { return t.cli.getOwnID() }

func (t sendTransport) OwnLID() types.JID { return t.cli.getOwnLID() }

func (t sendTransport) GenerateMessageID() types.MessageID { return t.cli.GenerateMessageID() }

// Errors entrega o MESMO ponteiro de sentinela da raiz. Ver o doc de
// send.Errors para por que isso importa.
func (t sendTransport) Errors() send.Errors {
	return send.Errors{NotLoggedIn: ErrNotLoggedIn}
}

func (t sendTransport) IsMessenger() bool { return t.cli.MessengerConfig != nil }

func (t sendTransport) AutoTrustIdentity() bool { return t.cli.AutoTrustIdentity }

func (t sendTransport) DefaultRequestTimeout() time.Duration { return defaultRequestTimeout }

// SendLock devolve o ponteiro para o mutex de envio do cliente. O ponteiro
// precisa ser estavel — messageSendLock e' campo de *Client e sendTransport
// embrulha o ponteiro do cliente, entao o mutex nunca e' copiado por valor.
func (t sendTransport) SendLock() *sync.Mutex { return &t.cli.messageSendLock }

func (t sendTransport) WaitResponse(reqID string) chan *waBinary.Node {
	return t.cli.waitResponse(reqID)
}

func (t sendTransport) CancelResponse(reqID string, ch chan *waBinary.Node) {
	t.cli.cancelResponse(reqID, ch)
}

func (t sendTransport) SendNodeAndGetData(ctx context.Context, node waBinary.Node) ([]byte, error) {
	return t.cli.sendNodeAndGetData(ctx, node)
}

func (t sendTransport) IsDisconnectNode(node *waBinary.Node) bool {
	return isDisconnectNode(node)
}

func (t sendTransport) RetryFrame(
	ctx context.Context,
	reqType, id string,
	data []byte,
	origResp *waBinary.Node,
	timeout time.Duration,
) (*waBinary.Node, error) {
	return t.cli.retryFrame(ctx, reqType, id, data, origResp, timeout)
}

func (t sendTransport) AddRecentMessage(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	wa *waE2E.Message,
	fb *waMsgApplication.MessageApplication,
) error {
	return t.cli.addRecentMessage(ctx, to, id, wa, fb)
}

func (t sendTransport) CachedGroupData(ctx context.Context, jid types.JID) (*group.Meta, error) {
	return t.cli.getCachedGroupData(ctx, jid)
}

func (t sendTransport) BroadcastListParticipants(ctx context.Context, jid types.JID) ([]types.JID, error) {
	return t.cli.getBroadcastListParticipants(ctx, jid)
}

func (t sendTransport) UserDevices(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	return t.cli.GetUserDevices(ctx, jids)
}

func (t sendTransport) UserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	return t.cli.GetUserInfo(ctx, jids)
}

func (t sendTransport) FetchPreKeysNoError(ctx context.Context, devices []types.JID) map[types.JID]*prekey.Bundle {
	return t.cli.fetchPreKeysNoError(ctx, devices)
}

func (t sendTransport) InvalidateGroupCache(jid types.JID) { t.cli.groupCache.Delete(jid) }

func (t sendTransport) InvalidateDeviceCache(jid types.JID) { t.cli.userDevicesCache.Delete(jid) }

func (t sendTransport) MigrateSessionStore(ctx context.Context, pn, lid types.JID) {
	t.cli.migrateSessionStore(ctx, pn, lid)
}

func (t sendTransport) ClearUntrustedIdentity(ctx context.Context, target types.JID) error {
	return t.cli.clearUntrustedIdentity(ctx, target)
}

func (t sendTransport) ShouldIncludeReportingToken(message *waE2E.Message) bool {
	return t.cli.shouldIncludeReportingToken(message)
}

func (t sendTransport) MessageReportingToken(
	msgProtobuf []byte,
	msg *waE2E.Message,
	senderJID, remoteJID types.JID,
	messageID types.MessageID,
) waBinary.Node {
	return t.cli.getMessageReportingToken(msgProtobuf, msg, senderJID, remoteJID, messageID)
}

func (t sendTransport) ApplyBotMessageHKDF(messageSecret []byte) []byte {
	return applyBotMessageHKDF(messageSecret)
}

func (t sendTransport) EnsureTCToken(ctx context.Context, jid types.JID) ([]byte, error) {
	return t.cli.ensureTCToken(ctx, jid)
}

func (t sendTransport) ResolveTCTokenStorageLID(ctx context.Context, jid types.JID) types.JID {
	return t.cli.resolveTCTokenStorageLID(ctx, jid)
}

func (t sendTransport) TCTokenSenderTS(jid types.JID) time.Time {
	return t.cli.getTCTokenSenderTS(jid)
}

func (t sendTransport) IssuePrivacyTokenAndSave(jid types.JID, senderTimestamp time.Time) {
	t.cli.issuePrivacyTokenAndSave(jid, senderTimestamp)
}

func (t sendTransport) GenerateCsToken(ctx context.Context, jid types.JID) []byte {
	return t.cli.generateCsToken(ctx, jid)
}
