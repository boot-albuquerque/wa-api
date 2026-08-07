package user

// codeUserSessionUnavailable e devolvido pelos tres metodos de
// ContactDirectory quando nao ha sessao ativa; o caller distingue este caso
// de uma falha do SDK pelo codigo, nao pela mensagem.
const codeUserSessionUnavailable = "user_session_unavailable"

// codeUserInfoTargetsInvalid e devolvido quando os JIDs do PEDIDO nao parseiam.
// E erro de entrada do chamador, nao falha nossa — ver o comentario em
// UserAdapter.GetUserInfo.
const codeUserInfoTargetsInvalid = "user_info_targets_invalid"
