package whatsmeow

const (
	// codeSessionAlreadyPaired é devolvido por dois caminhos distintos de
	// Pair (checagem local de credenciais e ErrQRStoreContainsID do SDK) que
	// o caller precisa tratar da mesma forma.
	codeSessionAlreadyPaired = "session_already_paired"

	// codeUserSessionUnavailable e devolvido pelos tres metodos de
	// ContactDirectory quando nao ha sessao ativa; o caller distingue este
	// caso de uma falha do SDK pelo codigo, nao pela mensagem.
	codeUserSessionUnavailable = "user_session_unavailable"
)
