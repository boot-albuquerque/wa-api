package message

import "errors"

// Os sentinelas abaixo sao os que a auditoria do lote 9 confirmou serem
// EXCLUSIVOS do caminho de recepcao/segredo de mensagem: fora de errors.go (a
// raiz, que os reexporta) e dos testes, nenhum arquivo alem dos 11 do lote os
// cita.
//
// A raiz continua expondo os nomes historicos, mas por ATRIBUICAO — o mesmo
// valor, nao copias. Um `errors.New` proprio na raiz quebraria `errors.Is` para
// quem comparasse com o nome historico. Mesmo racional de send/errors.go
// (lote 8), travado por teste.
var (
	ErrOriginalMessageSecretNotFound = errors.New("original message secret key not found")
	ErrNotEncryptedReactionMessage   = errors.New("given message isn't an encrypted reaction message")
	ErrNotEncryptedCommentMessage    = errors.New("given message isn't an encrypted comment message")
	ErrNotSecretEncryptedMessage     = errors.New("given message isn't a secret encrypted message")
	ErrNotPollUpdateMessage          = errors.New("given message isn't a poll update message")
)

// ErrEventAlreadyProcessed marca uma mensagem cujo ciphertext ja' estava no
// buffer de eventos decifrados com plaintext nulo — ou seja, ja' foi entregue
// aos handlers antes. O chamador a IGNORA em silencio (Debugf, `continue`), e
// nao a trata como falha de decifragem: mandar retry receipt aqui faria o
// telefone reenviar uma mensagem ja' processada.
//
// A raiz a reexporta pelo nome historico `EventAlreadyProcessed` (sem o
// prefixo `Err`), por atribuicao.
var ErrEventAlreadyProcessed = errors.New("event was already processed")

// errUnexpectedOrigSenderServer indica que o participante da MessageKey do
// grupo nao esta' nem em @s.whatsapp.net nem em @lid — servidor que este
// pacote nao sabe usar como remetente original.
var errUnexpectedOrigSenderServer = errors.New("unexpected server")
