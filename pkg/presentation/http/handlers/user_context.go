package handlers

// userInfo e a forma minima que os handlers exigem do valor que o middleware
// de autenticacao guarda no contexto sob appport.UserInfoKey. O valor concreto
// e bootstrap.Values; os handlers so precisam ler campos por nome, e declarar
// isso como interface local mantem pkg/presentation independente de
// pkg/bootstrap.
type userInfo interface {
	Get(key string) string
}
