package user

import "wa-api/pkg/domain"

// ErrDuplicateToken indica que o token pedido já pertence a outro usuário.
//
// É o MESMO valor de domain.ErrDuplicateToken, não uma cópia: quem o produz
// agora é o adapter de persistência, e handlers e testes que já faziam
// errors.Is contra este símbolo continuam funcionando sem mudança.
var ErrDuplicateToken = domain.ErrDuplicateToken

// ErrAvatarNotFound é o MESMO valor de domain.ErrAvatarNotFound — permite ao
// handler distinguir "sem foto pública" (não é erro) de falha real do
// GetAvatarUseCase sem importar domain diretamente.
var ErrAvatarNotFound = domain.ErrAvatarNotFound

// ErrAvatarUnauthorized é o MESMO valor de domain.ErrAvatarUnauthorized —
// "tem foto mas está escondida por privacidade", distinto de "não tem foto".
var ErrAvatarUnauthorized = domain.ErrAvatarUnauthorized
