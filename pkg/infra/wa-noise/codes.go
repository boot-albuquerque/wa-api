package whatsmeow

const (
	// codeUserSessionUnavailable e devolvido pelos tres metodos de
	// ContactDirectory quando nao ha sessao ativa; o caller distingue este
	// caso de uma falha do SDK pelo codigo, nao pela mensagem.
	codeUserSessionUnavailable = "user_session_unavailable"
)
