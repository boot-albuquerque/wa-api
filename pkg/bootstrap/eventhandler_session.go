package bootstrap

import (
	"context"
	"fmt"
	"time"

	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// Eventos de ciclo de vida da sessão: conectar, parear, cair, ser derrubado,
// ser banido. É o único arquivo desta divisão cujos ramos escrevem no banco e
// abortam o disparo do webhook.
//
// Os handlers que devolvem bool traduzem o `return` do ramo original: false
// significa "aborte sem disparar webhook".

func (evh *UserEventHandler) handleAppStateSyncComplete(evt *events.AppStateSyncComplete, st *eventState) {
	if len(evh.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
		err := evh.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to send available presence")
		} else {
			log.Info().Msg("Marked self as available")
		}
	}

	// WAPatchCriticalUnblockLow carrega a agenda de contatos do usuário
	// (wa-api/internal/wa-noise/protocol/appstate.WAPatchCriticalUnblockLow). Observamos a
	// conclusão desse patch com uma contagem — não com os contatos em si —
	// porque é a mesma fonte que GET /user/contacts lê
	// (evh.WAClient.Store.Contacts.GetAllContacts), então o número aqui
	// correlaciona diretamente com o que qualquer chamador HTTP vê depois
	// desse sync.
	if evt.Name == appstate.WAPatchCriticalUnblockLow {
		contacts, err := evh.WAClient.Store.Contacts.GetAllContacts(context.Background())
		if err != nil {
			log.Warn().Str("userid", evh.UserID).Str("patch", string(evt.Name)).Err(err).Msg("Failed to get contact count after app state sync")
		} else {
			log.Info().Str("userid", evh.UserID).Str("patch", string(evt.Name)).Uint64("version", evt.Version).Int("contact_count", len(contacts)).Msg("Contact roster app state sync complete")
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
func (evh *UserEventHandler) handleConnected(st *eventState) bool {
	st.postmap["type"] = "Connected"
	st.dowebhook = 1
	if len(evh.WAClient.Store.PushName) == 0 {
		return true
	}
	// Send presence available when connecting and when the pushname is changed.
	// This makes sure that outgoing messages always have the right pushname.
	err := evh.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send available presence")
	} else {
		log.Info().Msg("Marked self as available")
	}
	sqlStmt := `UPDATE users SET connected=1 WHERE id=$1`
	_, err = evh.DB.Exec(sqlStmt, evh.UserID)
	if err != nil {
		log.Error().Err(err).Msg(sqlStmt)
		return false
	}
	return true
}

func (evh *UserEventHandler) handlePairSuccess(evt *events.PairSuccess, st *eventState) bool {
	log.Info().Str("userid", evh.UserID).Str("token", evh.Token).Str("ID", evt.ID.String()).Str("BusinessName", evt.BusinessName).Str("Platform", evt.Platform).Msg("QR Pair Success")
	jid := evt.ID
	sqlStmt := `UPDATE users SET jid=$1 WHERE id=$2`
	_, err := evh.DB.Exec(sqlStmt, jid, evh.UserID)
	if err != nil {
		log.Error().Err(err).Msg(sqlStmt)
		return false
	}

	st.postmap["type"] = "PairSuccess"
	st.dowebhook = 1

	myuserinfo, found := appCtx.UserInfoCache.Get(evh.Token)
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
	query = evh.DB.Rebind(query)
	err = evh.DB.Get(&daysToSyncHistory, query, evh.UserID)
	if err != nil {
		log.Warn().Err(err).Str("userID", evh.UserID).Msg("Failed to get days_to_sync_history from database")
	} else if daysToSyncHistory > 0 {
		// Trigger history sync in a goroutine to avoid blocking
		// Wait a bit for the connection to be fully established
		go evh.syncHistoryAfterPair(daysToSyncHistory)
	}
	return true
}

// syncHistoryAfterPair era o corpo da goroutine anônima disparada por
// PairSuccess. Continua rodando em goroutine própria — quem a chama é
// `go evh.syncHistoryAfterPair(...)` — e o único valor que a closure
// capturava além de evh, daysToSyncHistory, virou parâmetro. Nada escreve
// nessa variável depois do `go`, então capturar por referência e receber por
// valor produzem o mesmo número.
func (evh *UserEventHandler) syncHistoryAfterPair(daysToSyncHistory int) {
	time.Sleep(2 * time.Second) // Give WhatsApp time to fully establish connection

	log.Info().
		Str("userID", evh.UserID).
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
	contacts, err := evh.WAClient.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		log.Error().Err(err).Str("userID", evh.UserID).Msg("Failed to get contacts for history sync")
	} else {
		for jid := range contacts {
			chatJIDs = append(chatJIDs, jid.String())
		}
	}

	// Get all groups
	groups, err := evh.WAClient.GetJoinedGroups(ctx)
	if err != nil {
		log.Error().Err(err).Str("userID", evh.UserID).Msg("Failed to get groups for history sync")
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
		err = syncHistoryForChat(context.Background(), evh.DB, evh.UserID, chatJID, count)
		if err != nil {
			log.Warn().Err(err).Str("chatJID", chatJIDStr).Msg("Failed to sync history for chat")
		} else {
			log.Info().Str("chatJID", chatJIDStr).Int("count", count).Msg("History sync request sent for chat")
		}

		// Small delay between requests to avoid overwhelming WhatsApp
		time.Sleep(100 * time.Millisecond)
	}

	log.Info().
		Str("userID", evh.UserID).
		Int("days", daysToSyncHistory).
		Int("chatsSynced", len(chatJIDs)).
		Msg("Automatic history sync completed after QR code scan")
}

// handleStreamReplaced sempre aborta: o ramo original era um log seguido de
// `return`, sem tocar em dowebhook.
func (evh *UserEventHandler) handleStreamReplaced(evt *events.StreamReplaced, st *eventState) bool {
	log.Info().Msg("Received StreamReplaced event")
	return false
}

// handleLoggedOut NÃO contém o `defer` que sinaliza o KillChannel. Ele
// permanece no `case` de handleEvent, de propósito: `defer` adia até o fim
// da FUNÇÃO, não do case, então no arquivo original o sinal era emitido DEPOIS
// de sendEventWithWebHook. Trazê-lo para cá o anteciparia para antes do
// webhook — mudança de ordem que compila calada e que o plano nomeia como o
// risco desta fase.
func (evh *UserEventHandler) handleLoggedOut(evt *events.LoggedOut, st *eventState) bool {
	st.postmap["type"] = "LoggedOut"
	st.dowebhook = 1
	log.Info().Str("reason", evt.Reason.String()).Msg("Logged out")
	sqlStmt := `UPDATE users SET connected=0 WHERE id=$1`
	_, err := evh.DB.Exec(sqlStmt, evh.UserID)
	if err != nil {
		log.Error().Err(err).Msg(sqlStmt)
		return false
	}
	return true
}

func (evh *UserEventHandler) handleDisconnected(evt *events.Disconnected, st *eventState) {
	st.postmap["type"] = "Disconnected"
	st.dowebhook = 1
	log.Info().Str("reason", fmt.Sprintf("%+v", evt)).Msg("Disconnected from Whatsapp")
}

func (evh *UserEventHandler) handleConnectFailure(evt *events.ConnectFailure, st *eventState) {
	st.postmap["type"] = "ConnectFailure"
	st.dowebhook = 1
	log.Error().Str("reason", fmt.Sprintf("%+v", evt)).Msg("Failed to connect to Whatsapp")
}

func (evh *UserEventHandler) handleKeepAliveRestored(evt *events.KeepAliveRestored, st *eventState) {
	st.postmap["type"] = "KeepAliveRestored"
	st.dowebhook = 1
	log.Info().Msg("Keep alive restored")
}

func (evh *UserEventHandler) handleKeepAliveTimeout(evt *events.KeepAliveTimeout, st *eventState) {
	st.postmap["type"] = "KeepAliveTimeout"
	st.dowebhook = 1
	log.Warn().Msg("Keep alive timeout")
}

func (evh *UserEventHandler) handleClientOutdated(evt *events.ClientOutdated, st *eventState) {
	st.postmap["type"] = "ClientOutdated"
	st.dowebhook = 1
	log.Warn().Msg("Client outdated")
}

func (evh *UserEventHandler) handleTemporaryBan(evt *events.TemporaryBan, st *eventState) {
	st.postmap["type"] = "TemporaryBan"
	st.dowebhook = 1
	log.Info().Msg("Temporary ban")
}

func (evh *UserEventHandler) handleStreamError(evt *events.StreamError, st *eventState) {
	st.postmap["type"] = "StreamError"
	st.dowebhook = 1
	log.Error().Str("code", evt.Code).Msg("Stream error")
}

func (evh *UserEventHandler) handlePairError(evt *events.PairError, st *eventState) {
	st.postmap["type"] = "PairError"
	st.dowebhook = 1
	log.Error().Msg("Pair error")
}

func (evh *UserEventHandler) handleAppState(evt *events.AppState, st *eventState) {
	log.Info().Str("index", fmt.Sprintf("%+v", evt.Index)).Str("actionValue", fmt.Sprintf("%+v", evt.SyncActionValue)).Msg("App state event received")
}
