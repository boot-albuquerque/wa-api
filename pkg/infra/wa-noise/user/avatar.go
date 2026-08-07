package user

import (
	"context"
	"errors"
	wajid "wa-api/pkg/infra/wa-noise/jid"

	"wa-api/pkg/domain"

	wa "wa-api/internal/wa-noise"
)

// GetProfilePicture devolve o avatar de um contato, ou (nil, domain.ErrAvatar*)
// se não há.
//
// wa-noise distingue "sem foto" (ErrProfilePictureNotSet, resposta 404/
// item-not-found do servidor) de "foto escondida via privacidade"
// (ErrProfilePictureUnauthorized, 401/not-authorized) de um erro real de
// transporte/sessão — nenhum dos dois é falha, é o WhatsApp respondendo "não
// há nada pra ver aqui" por dois motivos distintos, e o caller (handler HTTP)
// precisa saber QUAL dos dois pra responder 404 vs 403. Antes esta função
// devolvia os dois como `err` genérico, que subia até GetAvatarUseCase como
// "failed to get avatar: %v" — indistinguível de falha real e respondido
// como 500 pelo handler. `pic == nil, err == nil` (abaixo) é o branch de
// ExistingID/If-Modified-Since do wa-noise; nunca ocorre aqui porque
// ExistingID é sempre "".
func (a *UserAdapter) GetProfilePicture(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(target)
	if err != nil {
		return nil, err
	}

	// ExistingID vazio: o use case sempre passava "".
	pic, err := client.GetProfilePictureInfo(ctx, jid, &wa.GetProfilePictureParams{
		Preview:    preview,
		ExistingID: "",
	})
	if errors.Is(err, wa.ErrProfilePictureNotSet) {
		return nil, domain.ErrAvatarNotFound
	}
	if errors.Is(err, wa.ErrProfilePictureUnauthorized) {
		return nil, domain.ErrAvatarUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if pic == nil {
		return nil, domain.ErrAvatarNotFound
	}
	return &domain.AvatarInfo{ID: pic.ID, URL: pic.URL}, nil
}
