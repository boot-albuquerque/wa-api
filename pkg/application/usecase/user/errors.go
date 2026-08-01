package user

import "wa-api/pkg/domain"

// ErrDuplicateToken indica que o token pedido já pertence a outro usuário.
//
// É o MESMO valor de domain.ErrDuplicateToken, não uma cópia: quem o produz
// agora é o adapter de persistência, e handlers e testes que já faziam
// errors.Is contra este símbolo continuam funcionando sem mudança.
var ErrDuplicateToken = domain.ErrDuplicateToken
