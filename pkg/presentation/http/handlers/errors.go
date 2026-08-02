package handlers

// Erros-sentinela compartilhados pelos handlers deste pacote. Todos descrevem
// falha de fronteira HTTP — o que faltou ou nao pode ser lido na requisicao —
// e nunca falha de dominio, que vem tipada dos use cases.
var (
	errUnauthorized     = &simpleErr{"unauthorized"}
	errMissingSessionID = &simpleErr{"missing session id"}
	errMissingID        = &simpleErr{"missing ID"}
	errDecodePayload    = &simpleErr{"could not decode payload"}
)

type simpleErr struct {
	msg string
}

func (e simpleErr) Error() string {
	return e.msg
}
