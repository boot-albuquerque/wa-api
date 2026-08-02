package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Eventos de ciclo de vida da sessão: conectar, parear, cair, ser derrubado,
// ser banido. É o único arquivo desta divisão cujos ramos escrevem no banco e
// abortam o disparo do webhook.
//
// Os handlers que devolvem bool traduzem o `return` do ramo original: false
// significa "aborte sem disparar webhook".

func (mycli *MyClient) handleAppStateSyncComplete(evt *events.AppStateSyncComplete, st *eventState) {
	if len(mycli.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
		err := mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to send available presence")
		} else {
			log.Info().Msg("Marked self as available")
		}
	}
}

// handleConnected atende events.Connected E events.PushNameSetting — o ramo
// original era um `case` de dois tipos, e por isso não recebe o evento: o
// corpo nunca o usou, só o estado do cliente.
//
// O `break` do ramo original (pushname vazio) vira `return true`: ele saía do
// switch com dowebhook já em 1, então o webhook era disparado. Trocá-lo por
// `return false` silenciaria o evento Connected de toda sessão que ainda não
// tem pushname.
func (mycli *MyClient) handleConnected(st *eventState) bool {
	st.postmap["type"] = "Connected"
	st.dowebhook = 1
	if len(mycli.WAClient.Store.PushName) == 0 {
		return true
	}
	// Send presence available when connecting and when the pushname is changed.
	// This makes sure that outgoing messages always have the right pushname.
	err := mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send available presence")
	} else {
		log.Info().Msg("Marked self as available")
	}
	sqlStmt := `UPDATE users SET connected=1 WHERE id=$1`
	_, err = mycli.DB.Exec(sqlStmt, mycli.UserID)
	if err != nil {
		log.Error().Err(err).Msg(sqlStmt)
		return false
	}
	return true
}

func (mycli *MyClient) handlePairSuccess(evt *events.PairSuccess, st *eventState) bool {
	log.Info().Str("userid", mycli.UserID).Str("token", mycli.Token).Str("ID", evt.ID.String()).Str("BusinessName", evt.BusinessName).Str("Platform", evt.Platform).Msg("QR Pair Success")
	jid := evt.ID
	sqlStmt := `UPDATE users SET jid=$1 WHERE id=$2`
	_, err := mycli.DB.Exec(sqlStmt, jid, mycli.UserID)
	if err != nil {
		log.Error().Err(err).Msg(sqlStmt)
		return false
	}

	st.postmap["type"] = "PairSuccess"
	st.dowebhook = 1

	myuserinfo, found := appCtx.UserInfoCache.Get(mycli.Token)
	if !found {
		log.Warn().Msg("No user info cached on pairing?")
	} else {
		st.txtid = myuserinfo.(Values).Get("Id")
		token := myuserinfo.(Values).Get("Token")
		v := updateUserInfo(myuserinfo, "Jid", jid.String())
		appCtx.UserInfoCache.Set(token, v, cache.NoExpiration)
		log.Info().Str("jid", jid.String()).Str("userid", st.txtid).Str("token", token).Msg("User information set")
	}

	// Check if automatic history sync is enabled and trigger it after QR code is scanned
	var daysToSyncHistory int
	query := "SELECT COALESCE(days_to_sync_history, 0) FROM users WHERE id=$1"
	query = mycli.DB.Rebind(query)
	err = mycli.DB.Get(&daysToSyncHistory, query, mycli.UserID)
	if err != nil {
		log.Warn().Err(err).Str("userID", mycli.UserID).Msg("Failed to get days_to_sync_history from database")
	} else if daysToSyncHistory > 0 {
		// Trigger history sync in a goroutine to avoid blocking
		// Wait a bit for the connection to be fully established
		go mycli.syncHistoryAfterPair(daysToSyncHistory)
	}
	return true
}

// syncHistoryAfterPair era o corpo da goroutine anônima disparada por
// PairSuccess. Continua rodando em goroutine própria — quem a chama é
// `go mycli.syncHistoryAfterPair(...)` — e o único valor que a closure
// capturava além de mycli, daysToSyncHistory, virou parâmetro. Nada escreve
// nessa variável depois do `go`, então capturar por referência e receber por
// valor produzem o mesmo número.
func (mycli *MyClient) syncHistoryAfterPair(daysToSyncHistory int) {
	time.Sleep(2 * time.Second) // Give WhatsApp time to fully establish connection

	log.Info().
		Str("userID", mycli.UserID).
		Int("days", daysToSyncHistory).
		Msg("Triggering automatic history sync after QR code scan")

	// Use the SyncWhatsAppHistory logic but for a single user
	// Calculate message count based on days (estimate: 15 messages per day)
	count := daysToSyncHistory * 15
	if count > 500 {
		count = 500 // WhatsApp limit
	}
	if count < 50 {
		count = 50 // Minimum reasonable count
	}

	// Get chats from WhatsApp (contacts and groups)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chatJIDs []string

	// Get all contacts
	contacts, err := mycli.WAClient.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		log.Error().Err(err).Str("userID", mycli.UserID).Msg("Failed to get contacts for history sync")
	} else {
		for jid := range contacts {
			chatJIDs = append(chatJIDs, jid.String())
		}
	}

	// Get all groups
	groups, err := mycli.WAClient.GetJoinedGroups(ctx)
	if err != nil {
		log.Error().Err(err).Str("userID", mycli.UserID).Msg("Failed to get groups for history sync")
	} else {
		for _, group := range groups {
			chatJIDs = append(chatJIDs, group.JID.String())
		}
	}

	// Sync each chat with a small delay between requests
	for _, chatJIDStr := range chatJIDs {
		chatJID, err := types.ParseJID(chatJIDStr)
		if err != nil {
			log.Warn().Err(err).Str("chatJID", chatJIDStr).Msg("Failed to parse chat JID, skipping")
			continue
		}

		// Use the syncHistoryForChat function from handlers.go
		err = syncHistoryForChat(context.Background(), mycli.DB, mycli.UserID, chatJID, count)
		if err != nil {
			log.Warn().Err(err).Str("chatJID", chatJIDStr).Msg("Failed to sync history for chat")
		} else {
			log.Info().Str("chatJID", chatJIDStr).Int("count", count).Msg("History sync request sent for chat")
		}

		// Small delay between requests to avoid overwhelming WhatsApp
		time.Sleep(100 * time.Millisecond)
	}

	log.Info().
		Str("userID", mycli.UserID).
		Int("days", daysToSyncHistory).
		Int("chatsSynced", len(chatJIDs)).
		Msg("Automatic history sync completed after QR code scan")
}

// handleStreamReplaced sempre aborta: o ramo original era um log seguido de
// `return`, sem tocar em dowebhook.
func (mycli *MyClient) handleStreamReplaced(evt *events.StreamReplaced, st *eventState) bool {
	log.Info().Msg("Received StreamReplaced event")
	return false
}

// handleLoggedOut NÃO contém o `defer` que sinaliza o KillChannel. Ele
// permanece no `case` de myEventHandler, de propósito: `defer` adia até o fim
// da FUNÇÃO, não do case, então no arquivo original o sinal era emitido DEPOIS
// de sendEventWithWebHook. Trazê-lo para cá o anteciparia para antes do
// webhook — mudança de ordem que compila calada e que o plano nomeia como o
// risco desta fase.
func (mycli *MyClient) handleLoggedOut(evt *events.LoggedOut, st *eventState) bool {
	st.postmap["type"] = "LoggedOut"
	st.dowebhook = 1
	log.Info().Str("reason", evt.Reason.String()).Msg("Logged out")
	sqlStmt := `UPDATE users SET connected=0 WHERE id=$1`
	_, err := mycli.DB.Exec(sqlStmt, mycli.UserID)
	if err != nil {
		log.Error().Err(err).Msg(sqlStmt)
		return false
	}
	return true
}

func (mycli *MyClient) handleDisconnected(evt *events.Disconnected, st *eventState) {
	st.postmap["type"] = "Disconnected"
	st.dowebhook = 1
	log.Info().Str("reason", fmt.Sprintf("%+v", evt)).Msg("Disconnected from Whatsapp")
}

func (mycli *MyClient) handleConnectFailure(evt *events.ConnectFailure, st *eventState) {
	st.postmap["type"] = "ConnectFailure"
	st.dowebhook = 1
	log.Error().Str("reason", fmt.Sprintf("%+v", evt)).Msg("Failed to connect to Whatsapp")
}

func (mycli *MyClient) handleKeepAliveRestored(evt *events.KeepAliveRestored, st *eventState) {
	st.postmap["type"] = "KeepAliveRestored"
	st.dowebhook = 1
	log.Info().Msg("Keep alive restored")
}

func (mycli *MyClient) handleKeepAliveTimeout(evt *events.KeepAliveTimeout, st *eventState) {
	st.postmap["type"] = "KeepAliveTimeout"
	st.dowebhook = 1
	log.Warn().Msg("Keep alive timeout")
}

func (mycli *MyClient) handleClientOutdated(evt *events.ClientOutdated, st *eventState) {
	st.postmap["type"] = "ClientOutdated"
	st.dowebhook = 1
	log.Warn().Msg("Client outdated")
}

func (mycli *MyClient) handleTemporaryBan(evt *events.TemporaryBan, st *eventState) {
	st.postmap["type"] = "TemporaryBan"
	st.dowebhook = 1
	log.Info().Msg("Temporary ban")
}

func (mycli *MyClient) handleStreamError(evt *events.StreamError, st *eventState) {
	st.postmap["type"] = "StreamError"
	st.dowebhook = 1
	log.Error().Str("code", evt.Code).Msg("Stream error")
}

func (mycli *MyClient) handlePairError(evt *events.PairError, st *eventState) {
	st.postmap["type"] = "PairError"
	st.dowebhook = 1
	log.Error().Msg("Pair error")
}

func (mycli *MyClient) handleAppState(evt *events.AppState, st *eventState) {
	log.Info().Str("index", fmt.Sprintf("%+v", evt.Index)).Str("actionValue", fmt.Sprintf("%+v", evt.SyncActionValue)).Msg("App state event received")
}
