// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

func msgNode(attrs waBinary.Attrs, children ...waBinary.Node) *waBinary.Node {
	n := &waBinary.Node{Tag: "message", Attrs: attrs}
	if len(children) > 0 {
		n.Content = children
	}
	return n
}

// --- parseMessageSource: pre-condicao ---

func TestParseMessageSourceNotLoggedIn(t *testing.T) {
	cli := &Client{Log: recvTestClient(t).Log, Store: &store.Device{}}
	_, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
		"from": recvTestOtherJID,
	}), false)
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}

// --- parseMessageSource: grupo ---

func TestParseMessageSourceGroup(t *testing.T) {
	cli := recvTestClient(t)
	src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
		"from":        recvTestGroupJID,
		"participant": recvTestOtherJID,
	}), true)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !src.IsGroup {
		t.Errorf("IsGroup = false")
	}
	if src.Chat != recvTestGroupJID {
		t.Errorf("Chat = %s", src.Chat)
	}
	if src.Sender != recvTestOtherJID {
		t.Errorf("Sender = %s", src.Sender)
	}
	if src.IsFromMe {
		t.Errorf("IsFromMe = true")
	}
}

// requireParticipant=false e' o caminho de recibo/notificacao: participant
// ausente nao pode virar erro.
func TestParseMessageSourceGroupOptionalParticipant(t *testing.T) {
	cli := recvTestClient(t)
	src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{"from": recvTestGroupJID}), false)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !src.Sender.IsEmpty() {
		t.Errorf("Sender = %s, queria vazio", src.Sender)
	}

	if _, err = cli.parseMessageSource(msgNode(waBinary.Attrs{"from": recvTestGroupJID}), true); err == nil {
		t.Fatal("requireParticipant=true sem participant devia falhar")
	}
}

// O atributo alternativo lido depende do addressing_mode: em modo LID o
// participant e' o LID e o alternativo e' o telefone, e vice-versa. Trocar os
// dois faz o par LID/PN ser gravado invertido.
func TestParseMessageSourceGroupAddressingMode(t *testing.T) {
	cli := recvTestClient(t)
	lid := types.NewJID("55443322", types.HiddenUserServer)

	t.Run("lid", func(t *testing.T) {
		src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
			"from":            recvTestGroupJID,
			"addressing_mode": string(types.AddressingModeLID),
			"participant":     lid,
			"participant_pn":  recvTestOtherJID,
			"participant_lid": types.NewJID("00000000", types.HiddenUserServer),
		}), true)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if src.AddressingMode != types.AddressingModeLID {
			t.Errorf("AddressingMode = %q", src.AddressingMode)
		}
		if src.SenderAlt != recvTestOtherJID {
			t.Fatalf("SenderAlt = %s, queria participant_pn %s", src.SenderAlt, recvTestOtherJID)
		}
	})

	t.Run("pn", func(t *testing.T) {
		src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
			"from":            recvTestGroupJID,
			"participant":     recvTestOtherJID,
			"participant_lid": lid,
			"participant_pn":  types.NewJID("5511000000000", types.DefaultUserServer),
		}), true)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if src.SenderAlt != lid {
			t.Fatalf("SenderAlt = %s, queria participant_lid %s", src.SenderAlt, lid)
		}
	})
}

// O JID alternativo costuma vir sem device; ele herda o device do Sender para
// que os dois enderecem o mesmo aparelho.
func TestParseMessageSourceSenderAltInheritsDevice(t *testing.T) {
	cli := recvTestClient(t)
	sender := recvTestOtherJID
	sender.Device = 3
	lid := types.NewJID("55443322", types.HiddenUserServer)

	src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
		"from":            recvTestGroupJID,
		"participant":     sender,
		"participant_lid": lid,
	}), true)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if src.SenderAlt.Device != 3 {
		t.Fatalf("SenderAlt.Device = %d, queria 3", src.SenderAlt.Device)
	}
}

// Se o alternativo ja' traz device proprio, ele manda.
func TestParseMessageSourceSenderAltKeepsOwnDevice(t *testing.T) {
	cli := recvTestClient(t)
	sender := recvTestOtherJID
	sender.Device = 3
	lid := types.NewJID("55443322", types.HiddenUserServer)
	lid.Device = 5

	src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
		"from":            recvTestGroupJID,
		"participant":     sender,
		"participant_lid": lid,
	}), true)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if src.SenderAlt.Device != 5 {
		t.Fatalf("SenderAlt.Device = %d, queria 5", src.SenderAlt.Device)
	}
}

func TestParseMessageSourceGroupFromMe(t *testing.T) {
	cli := recvTestClient(t)
	for _, own := range []types.JID{cli.getOwnID(), cli.getOwnLID()} {
		src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
			"from":        recvTestGroupJID,
			"participant": own,
		}), true)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if !src.IsFromMe {
			t.Errorf("participant %s: IsFromMe = false", own)
		}
	}
}

// --- parseMessageSource: broadcast ---

func TestParseMessageSourceBroadcastRecipients(t *testing.T) {
	cli := recvTestClient(t)
	lid := types.NewJID("55443322", types.HiddenUserServer)

	src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
		"from":        types.NewJID("status", types.BroadcastServer),
		"participant": cli.getOwnID(),
		"recipient":   recvTestOtherJID,
	}, waBinary.Node{Tag: "participants", Content: []waBinary.Node{
		// destinatario endereçado por LID
		{Tag: "to", Attrs: waBinary.Attrs{"jid": lid, "peer_recipient_pn": recvTestOtherJID}},
		// destinatario endereçado por telefone
		{Tag: "to", Attrs: waBinary.Attrs{"jid": recvTestOtherJID, "peer_recipient_lid": lid}},
		// filho que nao e' <to> tem que ser ignorado
		{Tag: "outro", Attrs: waBinary.Attrs{"jid": recvTestOtherJID}},
	}}), true)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !src.IsFromMe {
		t.Fatalf("IsFromMe = false")
	}
	if src.BroadcastListOwner != recvTestOtherJID {
		t.Errorf("BroadcastListOwner = %s", src.BroadcastListOwner)
	}
	if len(src.BroadcastRecipients) != 2 {
		t.Fatalf("%d destinatarios, queria 2 (o filho nao-<to> tem que ser ignorado)", len(src.BroadcastRecipients))
	}
	for i, r := range src.BroadcastRecipients {
		if r.LID != lid || r.PN != recvTestOtherJID {
			t.Errorf("destinatario %d = {LID:%s PN:%s}, queria {%s %s}", i, r.LID, r.PN, lid, recvTestOtherJID)
		}
	}
}

// A lista de destinatarios so' e' lida quando a mensagem e' nossa: para
// mensagem alheia o servidor nao manda (e nao devemos inventar) a lista.
func TestParseMessageSourceBroadcastRecipientsOnlyWhenFromMe(t *testing.T) {
	cli := recvTestClient(t)
	src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
		"from":        types.NewJID("status", types.BroadcastServer),
		"participant": recvTestOtherJID,
	}, waBinary.Node{Tag: "participants", Content: []waBinary.Node{
		{Tag: "to", Attrs: waBinary.Attrs{"jid": recvTestOtherJID}},
	}}), true)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if src.BroadcastRecipients != nil {
		t.Fatalf("BroadcastRecipients = %v, queria nil", src.BroadcastRecipients)
	}
}

// --- parseMessageSource: newsletter, proprio, terceiro, bot ---

func TestParseMessageSourceNewsletter(t *testing.T) {
	cli := recvTestClient(t)
	newsletter := types.NewJID("1234567", types.NewsletterServer)
	src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{"from": newsletter}), false)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if src.Chat != newsletter || src.Sender != newsletter {
		t.Fatalf("Chat = %s, Sender = %s", src.Chat, src.Sender)
	}
	if src.IsGroup {
		t.Errorf("IsGroup = true")
	}
}

// Mensagem do proprio usuario em outro aparelho: o chat vem do atributo
// recipient; sem ele, e' conversa consigo mesmo.
func TestParseMessageSourceFromOwnDevice(t *testing.T) {
	cli := recvTestClient(t)
	own := cli.getOwnID()
	own.Device = 2

	t.Run("com recipient", func(t *testing.T) {
		src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
			"from":      own,
			"recipient": recvTestOtherJID,
		}), false)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if !src.IsFromMe || src.Chat != recvTestOtherJID || src.Sender != own {
			t.Fatalf("src = %+v", src)
		}
	})

	t.Run("sem recipient", func(t *testing.T) {
		src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{"from": own}), false)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if src.Chat != own.ToNonAD() {
			t.Fatalf("Chat = %s, queria %s", src.Chat, own.ToNonAD())
		}
	})
}

// Os servers "hosted" sao normalizados para os canonicos antes de virar
// Chat/Sender; sem isso o mesmo contato apareceria como dois chats distintos.
func TestParseMessageSourceHostedServerNormalization(t *testing.T) {
	cli := recvTestClient(t)
	tests := []struct {
		name string
		from types.JID
		want string
	}{
		{"hosted pn", types.NewJID(recvTestOtherJID.User, types.HostedServer), types.DefaultUserServer},
		{"hosted lid", types.NewJID("55443322", types.HostedLIDServer), types.HiddenUserServer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{"from": tc.from}), false)
			if err != nil {
				t.Fatalf("erro: %v", err)
			}
			if src.Sender.Server != tc.want {
				t.Errorf("Sender.Server = %q, queria %q", src.Sender.Server, tc.want)
			}
			if src.Chat.Server != tc.want {
				t.Errorf("Chat.Server = %q, queria %q", src.Chat.Server, tc.want)
			}
		})
	}
}

func TestParseMessageSourceOtherUserAlt(t *testing.T) {
	cli := recvTestClient(t)
	lid := types.NewJID("55443322", types.HiddenUserServer)

	t.Run("de pn le sender_lid", func(t *testing.T) {
		src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
			"from":       recvTestOtherJID,
			"sender_lid": lid,
			"sender_pn":  types.NewJID("5511000000000", types.DefaultUserServer),
		}), false)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if src.SenderAlt != lid {
			t.Fatalf("SenderAlt = %s, queria %s", src.SenderAlt, lid)
		}
	})

	t.Run("de lid le sender_pn", func(t *testing.T) {
		src, err := cli.parseMessageSource(msgNode(waBinary.Attrs{
			"from":       lid,
			"sender_pn":  recvTestOtherJID,
			"sender_lid": types.NewJID("00000000", types.HiddenUserServer),
		}), false)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if src.SenderAlt != recvTestOtherJID {
			t.Fatalf("SenderAlt = %s, queria %s", src.SenderAlt, recvTestOtherJID)
		}
	})
}

// --- parseMsgBotInfo / parseMsgMetaInfo ---

// Os atributos de alvo da edicao so' sao lidos quando o edit type os exige;
// para os demais tipos o <bot> nem os traz e exigi-los daria erro de parse.
func TestParseMsgBotInfoEditTypes(t *testing.T) {
	cli := recvTestClient(t)
	for _, editType := range []types.BotEditType{types.EditTypeInner, types.EditTypeLast} {
		info, err := cli.parseMsgBotInfo(waBinary.Node{Content: []waBinary.Node{
			{Tag: "bot", Attrs: waBinary.Attrs{
				"edit":                string(editType),
				"edit_target_id":      "TARGET1",
				"sender_timestamp_ms": "1700000000000",
			}},
		}})
		if err != nil {
			t.Fatalf("%s: erro: %v", editType, err)
		}
		if info.EditType != editType {
			t.Errorf("EditType = %q", info.EditType)
		}
		if info.EditTargetID != "TARGET1" {
			t.Errorf("EditTargetID = %q", info.EditTargetID)
		}
		if info.EditSenderTimestampMS.UnixMilli() != 1700000000000 {
			t.Errorf("timestamp = %v", info.EditSenderTimestampMS)
		}
	}
}

func TestParseMsgBotInfoOtherEditTypeIgnoresTarget(t *testing.T) {
	cli := recvTestClient(t)
	info, err := cli.parseMsgBotInfo(waBinary.Node{Content: []waBinary.Node{
		{Tag: "bot", Attrs: waBinary.Attrs{"edit": "first"}},
	}})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.EditTargetID != "" {
		t.Errorf("EditTargetID = %q, queria vazio", info.EditTargetID)
	}
}

// deprecated_lid_session e' um *bool de tres estados: ausente tem que
// permanecer nil, para nao virar "false" explicito.
func TestParseMsgMetaInfoDeprecatedLIDSession(t *testing.T) {
	cli := recvTestClient(t)

	info, err := cli.parseMsgMetaInfo(waBinary.Node{Content: []waBinary.Node{
		{Tag: "meta", Attrs: waBinary.Attrs{}},
	}})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.DeprecatedLIDSession != nil {
		t.Fatalf("DeprecatedLIDSession = %v, queria nil quando ausente", *info.DeprecatedLIDSession)
	}

	for _, raw := range []string{"true", "false"} {
		info, err = cli.parseMsgMetaInfo(waBinary.Node{Content: []waBinary.Node{
			{Tag: "meta", Attrs: waBinary.Attrs{"deprecated_lid_session": raw}},
		}})
		if err != nil {
			t.Fatalf("%s: erro: %v", raw, err)
		}
		if info.DeprecatedLIDSession == nil {
			t.Fatalf("%s: DeprecatedLIDSession = nil", raw)
		}
		if got := *info.DeprecatedLIDSession; got != (raw == "true") {
			t.Errorf("%s: valor = %v", raw, got)
		}
	}
}

func TestParseMsgMetaInfoTargets(t *testing.T) {
	cli := recvTestClient(t)
	info, err := cli.parseMsgMetaInfo(waBinary.Node{Content: []waBinary.Node{
		{Tag: "meta", Attrs: waBinary.Attrs{
			"target_id":             "TARGET1",
			"target_sender_jid":     recvTestOtherJID,
			"target_chat_jid":       recvTestGroupJID,
			"thread_msg_id":         "THREAD1",
			"thread_msg_sender_jid": recvTestOtherJID,
		}},
	}})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.TargetID != "TARGET1" || info.TargetSender != recvTestOtherJID || info.TargetChat != recvTestGroupJID {
		t.Errorf("info = %+v", info)
	}
	if info.ThreadMessageID != "THREAD1" || info.ThreadMessageSenderJID != recvTestOtherJID {
		t.Errorf("thread = %q / %s", info.ThreadMessageID, info.ThreadMessageSenderJID)
	}
}

// Um <meta> ausente nao pode ser erro: a maioria das mensagens nao tem um.
func TestParseMsgMetaInfoMissingNode(t *testing.T) {
	cli := recvTestClient(t)
	info, err := cli.parseMsgMetaInfo(waBinary.Node{})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.TargetID != "" {
		t.Errorf("TargetID = %q", info.TargetID)
	}
}

// --- parseMessageInfo ---

func TestParseMessageInfoRequiredAttrs(t *testing.T) {
	cli := recvTestClient(t)
	info, err := cli.parseMessageInfo(msgNode(waBinary.Attrs{
		"from":        recvTestGroupJID,
		"participant": recvTestOtherJID,
		"id":          "MSG1",
		"t":           "1700000000",
		"server_id":   "7",
		"notify":      "Fulano",
		"category":    msgCategoryPeer,
		"type":        msgTypeText,
		"edit":        "1",
	}))
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.ID != "MSG1" || info.ServerID != 7 || info.Timestamp.Unix() != 1700000000 {
		t.Errorf("info = %+v", info)
	}
	if info.PushName != "Fulano" || info.Category != msgCategoryPeer || info.Type != msgTypeText {
		t.Errorf("info = %+v", info)
	}
	if info.Edit != types.EditAttribute("1") {
		t.Errorf("Edit = %q", info.Edit)
	}
}

// `id` e `t` sao obrigatorios: sem eles nao da' para mandar recibo nem ordenar
// a mensagem, entao o parse tem que falhar em vez de seguir com zeros.
func TestParseMessageInfoMissingRequiredAttrs(t *testing.T) {
	cli := recvTestClient(t)
	_, err := cli.parseMessageInfo(msgNode(waBinary.Attrs{
		"from":        recvTestGroupJID,
		"participant": recvTestOtherJID,
	}))
	if err == nil {
		t.Fatal("esperava erro por id/t ausentes")
	}
}

func TestParseMessageInfoChildren(t *testing.T) {
	cli := recvTestClient(t)
	info, err := cli.parseMessageInfo(msgNode(waBinary.Attrs{
		"from":        recvTestGroupJID,
		"participant": recvTestOtherJID,
		"id":          "MSG1",
		"t":           "1700000000",
	},
		waBinary.Node{Tag: "multicast"},
		waBinary.Node{Tag: "bot", Attrs: waBinary.Attrs{"edit": "1"}},
		waBinary.Node{Tag: "meta", Attrs: waBinary.Attrs{"target_id": "TARGET1"}},
		waBinary.Node{Tag: "franking"},
		waBinary.Node{Tag: "trace"},
		waBinary.Node{Tag: encNodeTag, Attrs: waBinary.Attrs{"mediatype": "image"}},
	))
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !info.Multicast {
		t.Errorf("Multicast = false")
	}
	if info.MsgMetaInfo.TargetID != "TARGET1" {
		t.Errorf("TargetID = %q", info.MsgMetaInfo.TargetID)
	}
	// mediatype e' colhido de qualquer filho nao reconhecido que o traga.
	if info.MediaType != "image" {
		t.Errorf("MediaType = %q", info.MediaType)
	}
}

// Um <bot>/<meta> mal formado so' pode virar warning: a mensagem em si continua
// processavel, e derrubar o parse aqui perderia a mensagem inteira.
func TestParseMessageInfoBadChildIsNotFatal(t *testing.T) {
	cli := recvTestClient(t)
	info, err := cli.parseMessageInfo(msgNode(waBinary.Attrs{
		"from":        recvTestGroupJID,
		"participant": recvTestOtherJID,
		"id":          "MSG1",
		"t":           "1700000000",
	},
		// <bot> sem o atributo obrigatorio `edit`
		waBinary.Node{Tag: "bot"},
	))
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.ID != "MSG1" {
		t.Errorf("ID = %q", info.ID)
	}
}
