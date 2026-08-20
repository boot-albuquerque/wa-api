package bootstrap

import (
	waE2E "wa-api/internal/wa-noise/protocol/proto/waE2E"
)

// A classificação de mensagem recebida, UMA vez, para os dois caminhos de
// ingestão.
//
// HOUSEKEEP F187. Havia duas cadeias — `eventhandler_message.go` para o que
// chega agora, `eventhandler_history.go` para o que vem por sincronização — e
// elas divergiram, como duas fontes de verdade sempre acabam por divergir. O
// custo medido em campo:
//
//   - o caminho de tempo real DESCARTAVA o texto que o próprio ramo extraía:
//     `DisplayName="Contato Varredura"` era gravado como `:contact:`, com o
//     nome intacto no `datajson`;
//   - e reconhecia MENOS tipos: `buttons_response` e `list_response` só
//     existiam no de sync.
//
// Depois de a F187 ser corrigida a divergência INVERTEU-SE — o de tempo real
// passou a ter seis ramos que faltavam ao de sync (protocolo, enquete,
// interativa, template, lista, botões). Isso é a prova de que remendar
// qualquer um dos lados não resolve: a única forma de os dois pararem de
// divergir é não haver dois.
//
// POR QUE ISTO ELIMINA A CLASSE DE DEFEITO, e não só a ocorrência. A função
// devolve o texto FINAL. Não há `caption` e `textContent` a competirem, e
// portanto não há um bloco a jusante que possa sobrescrever o que um ramo
// atribuiu — que era exatamente o mecanismo da F187. O defeito deixa de ser
// possível de escrever, em vez de ser corrigido caso a caso.

// messageClassification é o que uma mensagem recebida É, do ponto de vista do
// histórico: o tipo, o texto que o cliente lê, e a mensagem a que responde.
type messageClassification struct {
	Type string
	Text string
	// QuotedID é a mensagem referenciada: a original de uma edição, a reagida
	// de uma reação, a citada de uma resposta.
	QuotedID string
	// DeletedID é preenchido só para `delete`, e existe separado de QuotedID
	// porque o caminho histórico gravava o id apagado no TEXTO. Preservar isso
	// é contrato; misturá-lo com QuotedID mudaria o que o cliente lê.
	DeletedID string
}

// classifyMessage identifica a mensagem. `msg` tem de vir JÁ DESEMBRULHADO —
// isto é, de `events.Message.Message` depois de `UnwrapRaw`, e não de
// `RawMessage` (HOUSEKEEP F188).
//
// A ordem dos ramos é significativa e não é alfabética: `protocolMessage` vem
// primeiro porque apagar e editar são mensagens de protocolo que TAMBÉM
// carregam conteúdo, e testá-las depois faria uma edição ser classificada pelo
// conteúdo que ela edita.
//
// A ISENÇÃO do gate de cobertura de log abaixo é a PRIMEIRA do repositório, e a
// justificação é a razão de a válvula existir: esta função não tem modo de
// falha — não devolve erro, não chama nada que possa falhar, só olha para um
// struct e devolve outro. E corre no caminho de CADA mensagem recebida. Um log
// aqui não teria nada de útil a dizer e apareceria milhares de vezes por dia,
// que é a F180 na sua pior forma.
//
// As alternativas eram piores: inventar um log destrói o próprio log; baixar o
// piso de func_coverage desliga a proteção para todas as outras funções.
//
// Quem quiser remover esta isenção tem de responder à pergunta que ela evita:
// o que é que este log diria que o operador pudesse USAR?
//
//log:exempt classificacao pura, sem modo de falha, no caminho de CADA mensagem
func classifyMessage(msg *waE2E.Message) messageClassification {
	switch {
	case msg.GetProtocolMessage() != nil && msg.GetProtocolMessage().GetType() == waE2E.ProtocolMessage_REVOKE:
		pm := msg.GetProtocolMessage()
		return messageClassification{
			Type: messageTypeDelete,
			// O id apagado vai no TEXTO porque é onde o caminho histórico o
			// punha. É contrato, não conveniência.
			Text:      pm.GetKey().GetID(),
			DeletedID: pm.GetKey().GetID(),
		}

	case msg.GetProtocolMessage().GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT:
		pm := msg.GetProtocolMessage()
		return messageClassification{Type: messageTypeEdit, Text: editedText(pm), QuotedID: pm.GetKey().GetID()}

	case msg.GetReactionMessage() != nil:
		r := msg.GetReactionMessage()
		return messageClassification{Type: messageTypeReaction, Text: r.GetText(), QuotedID: r.GetKey().GetID()}

	case msg.GetImageMessage() != nil:
		return comTexto(messageTypeImage, msg.GetImageMessage().GetCaption())
	case msg.GetVideoMessage() != nil:
		return comTexto(messageTypeVideo, msg.GetVideoMessage().GetCaption())
	case msg.GetAudioMessage() != nil:
		return comTexto(messageTypeAudio, "")
	case msg.GetDocumentMessage() != nil:
		return comTexto(messageTypeDocument, msg.GetDocumentMessage().GetCaption())
	case msg.GetStickerMessage() != nil:
		return comTexto(messageTypeSticker, "")
	case msg.GetContactMessage() != nil:
		return comTexto(messageTypeContact, msg.GetContactMessage().GetDisplayName())
	case msg.GetLocationMessage() != nil:
		return comTexto(messageTypeLocation, msg.GetLocationMessage().GetName())

	case msg.GetButtonsResponseMessage() != nil:
		return comTexto(messageTypeButtonsResponse, msg.GetButtonsResponseMessage().GetSelectedButtonID())
	case msg.GetListResponseMessage() != nil:
		return comTexto(messageTypeListResponse, msg.GetListResponseMessage().GetSingleSelectReply().GetSelectedRowID())

	case msg.GetPollCreationMessage() != nil:
		return comTexto(messageTypePoll, msg.GetPollCreationMessage().GetName())
	case msg.GetInteractiveMessage() != nil:
		return comTexto(messageTypeButtons, msg.GetInteractiveMessage().GetBody().GetText())
	case msg.GetTemplateMessage() != nil:
		return comTexto(messageTypeTemplate, templateText(msg.GetTemplateMessage()))
	case msg.GetListMessage() != nil:
		return comTexto(messageTypeList, listText(msg.GetListMessage()))
	case msg.GetButtonsMessage() != nil:
		return comTexto(messageTypeButtons, msg.GetButtonsMessage().GetContentText())

	// F184 residual. Os tipos abaixo chegavam e eram DESCARTADOS: nenhum ramo
	// casava, o texto ficava vazio, e a guarda de gravação largava a linha —
	// com rasto desde a F186, mas largava.
	//
	// Acrescentá-los aqui custa uma linha cada PORQUE a cadeia foi unificada.
	// Antes eram dois sítios, e acrescentar num só teria criado a divergência
	// que a F187 acabou de fechar.
	case msg.GetPollUpdateMessage() != nil:
		// O voto numa enquete. O conteúdo vem cifrado ponta-a-ponta e é
		// decifrado noutro ponto do pipeline; aqui fica o marcador, que é
		// honesto — a linha passa a existir, e diz o que é.
		return comTexto(messageTypePollUpdate, "")
	case msg.GetInteractiveResponseMessage() != nil:
		return comTexto(messageTypeInteractiveResponse, "")
	case msg.GetEventMessage() != nil:
		return comTexto(messageTypeEvent, msg.GetEventMessage().GetName())
	case msg.GetLiveLocationMessage() != nil:
		return comTexto(messageTypeLiveLocation, msg.GetLiveLocationMessage().GetCaption())
	case msg.GetPtvMessage() != nil:
		// Vídeo circular curto. Tipo próprio e não `video`: são coisas
		// diferentes na interface do WhatsApp, e colapsá-las faria o cliente
		// perder a distinção.
		return comTexto(messageTypePtv, msg.GetPtvMessage().GetCaption())
	case msg.GetGroupInviteMessage() != nil:
		return comTexto(messageTypeGroupInvite, msg.GetGroupInviteMessage().GetGroupName())
	case msg.GetOrderMessage() != nil:
		return comTexto(messageTypeOrder, msg.GetOrderMessage().GetMessage())
	case msg.GetProductMessage() != nil:
		// O snapshot de produto NÃO tem título — tem descrição. Verificado nos
		// getters do proto, e não presumido: a primeira versão desta linha
		// usava GetTitle() e não compilava.
		return comTexto(messageTypeProduct, msg.GetProductMessage().GetProduct().GetDescription())
	case msg.GetContactsArrayMessage() != nil:
		return comTexto(messageTypeContactsArray, msg.GetContactsArrayMessage().GetDisplayName())
	}

	// Texto simples. `Conversation` e `ExtendedTextMessage` são as duas formas
	// que uma mensagem de texto toma, e a segunda é a que carrega a citação —
	// razão de o `QuotedID` só aparecer aqui e nos ramos de protocolo/reação.
	if c := msg.GetConversation(); c != "" {
		return messageClassification{Type: messageTypeText, Text: c}
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return messageClassification{
			Type:     messageTypeText,
			Text:     ext.GetText(),
			QuotedID: ext.GetContextInfo().GetStanzaID(),
		}
	}

	// Nada reconhecido. O `Type` fica em `text` e o `Text` vazio, que é a
	// combinação que a guarda de gravação usa para descartar — e que a F186
	// faz registar antes de descartar.
	return messageClassification{Type: messageTypeText}
}

// comTexto aplica o marcador quando o tipo tem um e o texto veio vazio.
//
// O marcador é do TIPO, não do conteúdo: `:image:` diz "há uma imagem aqui, sem
// legenda", que é informação. Texto vazio diria "não há nada", que é falso e faz
// a guarda de gravação descartar a linha.
func comTexto(tipo, texto string) messageClassification {
	if texto == "" {
		texto = defaultHistoryTextFor(tipo, "")
	}
	return messageClassification{Type: tipo, Text: texto}
}
