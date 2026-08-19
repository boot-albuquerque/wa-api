package handlers

import (
	"net/http"
)

// Este arquivo cobria de forma exaustiva os handlers de mensagem que ainda
// usavam port.MessageComposer. send/text (SendMessage) migrou para
// port.TextMessenger no CAP-01 e tem tabela própria em
// handler_message_send_test.go.
//
// send/edit e delete/message saíram desta tabela em CAP-10 (port.ChatMessenger
// + port.JIDResolver); send/template no CAP-15, send/buttons no CAP-21 e
// send/list — o ÚLTIMO caso que restava — no CAP-22 (todos port.SimpleMessenger
// ou port.InteractiveMessenger, envio de verdade). Os eixos desta tabela para
// send/list foram realocados, nome por nome, para handler_send_list_test.go:
//
//	sucesso                     TestSendList_Success_ViaRegisteredRoute
//	não autenticado             TestSendList_RejectUnauthenticated
//	tipo errado no contexto     TestSendList_WrongTypeInContext_ViaRegisteredRoute
//	session id vazio            TestSendList_MissingSessionID_ViaRegisteredRoute
//	corpo malformado            TestSendList_MalformedBody_ViaRegisteredRoute
//	campo obrigatório ausente   TestSendList_RejectMissingRequiredField
//	falha de sessão             TestSendList_SessionFailure
//	sucesso sem log de saída    TestSendList_SuccessEmitsNoOutcomeLog
//	segredo no log              TestSendList_NoSecretLeak
//	Id do cliente               TestSendList_ClientSuppliedIDIsForwardedButServerIDWins
//
// Os DOIS eixos de geração de ID desta tabela (MessageIDFailure e a metade
// `generatesID` de Success) não têm destino porque deixaram de existir para
// a rota: o use case não chama mais NewMessageID, e o MessageID publicado é
// o que a porta devolveu.
//
// Com send/list migrado, a tabela ficaria vazia — pior que removê-la, porque
// cada `for` passaria a iterar sobre zero casos e a suíte ficaria verde sem
// medir nada. O que sobra aqui é `msgAuthed`, usado por praticamente toda a
// suíte de send/* deste pacote — não apagar por engano ao tirar o último
// caso local.

// msgAuthed e' a mutacao padrao: requisicao autenticada com sessao valida.
func msgAuthed(r *http.Request) *http.Request { return withUser(r, "user-1") }
