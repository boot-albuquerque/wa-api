// Package newsletter adapts the page's channel capability to the application's
// NewsletterReader port.
//
// # The freshness caveat, which the caller has to know
//
// H123 proved the listing non-empty — one entry with membership=owner right
// after creating a channel, zero after deleting it. But H139 measured something
// the proof does not cover: `Followed` reflects the CLIENT'S CACHE, not the
// server. When ANOTHER account deletes a channel this one is admin of or
// subscribed to, the local model STAYS — six leftovers were found carrying
// serverAlive:false.
//
// This adapter does NOT filter them, and that is deliberate rather than lazy:
// the capability's DirectoryEntry carries no liveness field, so filtering here
// would mean inventing a judgement from data that is not there. Returning the
// cache as the cache is honest; returning a filtered list would claim a
// freshness this transport cannot deliver.
//
// It is a fourth kind of divergence, after "no meaning", "human dependency" and
// "missing datum": the answer is COMPLETE, and its FRESHNESS is not guaranteed.
// The port has nowhere to say so, so it is said here.
package newsletter

import (
	"context"
	"errors"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	adapter "wa-api/pkg/infra/wa-headless"
)

const (
	listLabel   = "adapter/list-subscribed"
	createLabel = "adapter/create-newsletter"
)

// follower is the slice of the page capability this adapter uses.
type follower interface {
	Followed(ctx context.Context, label string) ([]waheadless.ChannelEntry, error)
}

type creator interface {
	Create(ctx context.Context, name, description, label string) (waheadless.ChannelCreated, error)
}

// Reader implements appport.NewsletterReader over a headless session.
type Reader struct {
	sessions *adapter.Sessions
	// newFollower is overridable in tests. Nil uses the real page capability.
	newFollower func(ctx context.Context, txtID string) (follower, error)
	newCreator  func(ctx context.Context, txtID string) (creator, error)
}

// NewReader builds the adapter.
func NewReader(sessions *adapter.Sessions) *Reader { return &Reader{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (r *Reader) EnsureSession(ctx context.Context, txtID string) error {
	return r.sessions.EnsureSession(ctx, txtID)
}

// ListSubscribed returns the channels this account follows, AS THE CLIENT HOLDS
// THEM. See the package doc: entries can outlive the channel on the server.
func (r *Reader) ListSubscribed(ctx context.Context, txtID string) (any, error) {
	f, err := r.follower(ctx, txtID)
	if err != nil {
		return nil, err
	}
	entries, err := f.Followed(ctx, listLabel)
	if err != nil {
		return nil, err
	}
	// An empty listing is a legitimate answer — this account follows nothing —
	// and it is returned as an empty slice rather than nil so a caller that
	// ranges over it does not have to distinguish the two.
	if entries == nil {
		entries = []waheadless.ChannelEntry{}
	}
	return entries, nil
}

func (r *Reader) follower(ctx context.Context, txtID string) (follower, error) {
	if r.newFollower != nil {
		return r.newFollower(ctx, txtID)
	}
	eval, err := r.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewChannelManager(r.sessions.Runner(), eval), nil
}

// CreateNewsletter creates a channel.
//
// A FOTO É RECUSADA, e a recusa é medida, não preguiça: os módulos de foto
// deste build estão ausentes (`WAWebSetPicture` e `WAWebProfilePicThumbBridge`,
// H140/decisão 60), e a capability da página não tem por onde a receber.
//
// Aceitar os bytes e criar o canal sem eles seria o pior desfecho: o chamador
// veria sucesso e um canal sem imagem, sem nada dizer que a metade visual do
// que ele pediu não aconteceu. Recusar custa uma chamada; o silêncio custa a
// confiança em todas as outras.
//
// Sem foto, cria — e devolve o que a página devolveu, incluindo o código de
// convite, que é o dado pelo qual alguém entra no canal.
func (r *Reader) CreateNewsletter(ctx context.Context, txtID, name, description string, picture []byte) (any, error) {
	if name == "" {
		return nil, errNoName
	}
	if len(picture) > 0 {
		return nil, fmt.Errorf("waheadless: criar canal COM foto esta bloqueado neste build "+
			"(modulos de foto ausentes, medido em H140); %d bytes recusados em vez de descartados em silencio", len(picture))
	}
	c, err := r.creator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return c.Create(ctx, name, description, createLabel)
}

func (r *Reader) creator(ctx context.Context, txtID string) (creator, error) {
	if r.newCreator != nil {
		return r.newCreator(ctx, txtID)
	}
	eval, err := r.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewChannelManager(r.sessions.Runner(), eval), nil
}

// errNoName recusa um canal sem nome antes de a pagina o ver.
var errNoName = errors.New("waheadless: canal sem nome")

// NÃO HÁ asserção de tipo para appport.NewsletterReader aqui, e a ausência é
// deliberada — mesma disciplina do groupmembers antes da decisão 92.
//
// A porta cresceu para TREZE métodos (a fusão de feature/wa-noise trouxe
// mensagens de canal, atualizações, visualização, reação e assinatura de
// atualizações ao vivo). A capability da página oferece OITO, e só parte deles
// corresponde. Declarar que este adaptador satisfaz a porta seria a mentira que
// a decisão 80 existe para impedir.
//
// O que ele satisfaz hoje, e é verdade verificável: ListSubscribed e
// CreateNewsletter (sem foto, ver acima). O resto é fatia por medir — e a
// costura, quando vier, é a mesma da 92: partir a porta por capacidade de
// transporte, com o inventário da Fase 3 a acusar cada metade.
