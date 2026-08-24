// Package groupdir adapts group READING to the application's GroupDirectory
// port.
//
// É a primeira fatia que atravessa DUAS capabilities, e a razão é medida: a
// página não tem uma coleção de grupos separada. Um grupo é uma CONVERSA cujo
// jid termina em @g.us, então listar grupos é filtrar a lista de conversas — e
// o título vem de `formattedTitle`, não de `getName`, porque `getName` responde
// para 1 de 384 conversas (é a agenda, e num aparelho companheiro ela está
// quase vazia).
//
// O convite, esse sim, vem da capability de grupo.
package groupdir

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
)

const (
	listLabel   = "adapter/list-joined-groups"
	inviteLabel = "adapter/group-invite"
	infoLabel   = "adapter/group-info"

	// semLimite pede a lista INTEIRA. O port promete os grupos desta conta, e
	// truncar sem dizer seria a incompletude silenciosa que o próprio tipo
	// List existe para tornar visível.
	semLimite = 0
)

// lister e inviter são as fatias das capabilities que este adaptador usa.
type lister interface {
	List(ctx context.Context, limit int, label string) (waheadless.ChatList, error)
	ByJID(ctx context.Context, jid, label string) (waheadless.Chat, error)
}

type inviter interface {
	InviteCode(ctx context.Context, groupJID, label string) (waheadless.GroupInviteCode, error)
	InviteInfo(ctx context.Context, code, label string) (waheadless.GroupInvite, error)
}

// Directory implements appport.GroupDirectory over a headless session.
type Directory struct {
	sessions *adapter.Sessions
	// Sobrescritíveis em teste. Nil usa as capabilities reais.
	newLister  func(ctx context.Context, txtID string) (lister, error)
	newInviter func(ctx context.Context, txtID string) (inviter, error)
}

// NewDirectory builds the adapter.
func NewDirectory(sessions *adapter.Sessions) *Directory { return &Directory{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (d *Directory) EnsureSession(ctx context.Context, txtID string) error {
	return d.sessions.EnsureSession(ctx, txtID)
}

// GetGroupInfo reads one group by JID.
func (d *Directory) GetGroupInfo(ctx context.Context, txtID string, groupJID domain.JID) (any, error) {
	pageJID, err := adapter.ToPageJID(groupJID)
	if err != nil {
		return nil, err
	}
	l, err := d.lister(ctx, txtID)
	if err != nil {
		return nil, err
	}
	chat, err := l.ByJID(ctx, pageJID, infoLabel)
	if err != nil {
		return nil, err
	}
	// Um jid de grupo que devolve uma conversa NÃO-grupo é resposta errada com
	// forma certa: acontece se o chamador passar o jid de uma pessoa. Recusar
	// aqui é mais barato que deixar o chamador tratar uma pessoa como grupo.
	if !chat.IsGroup {
		return nil, fmt.Errorf("waheadless: %q is not a group conversation", pageJID)
	}
	return chat, nil
}

// ListJoinedGroups returns the groups this account is in, and how many.
//
// A contagem é a dos GRUPOS devolvidos, e não o total de conversas: a página
// não separa grupos numa coleção própria, então a lista vem filtrada daqui — e
// devolver o total de conversas faria o chamador acreditar que está em centenas
// de grupos.
func (d *Directory) ListJoinedGroups(ctx context.Context, txtID string) (any, int, error) {
	grupos, err := d.grupos(ctx, txtID)
	if err != nil {
		return nil, 0, err
	}
	return grupos, len(grupos), nil
}

// GroupNames maps each group's JID to the title the app itself renders.
func (d *Directory) GroupNames(ctx context.Context, txtID string) (map[domain.JID]string, error) {
	grupos, err := d.grupos(ctx, txtID)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.JID]string, len(grupos))
	for _, g := range grupos {
		out[domain.JID(g.JID)] = g.Title
	}
	return out, nil
}

// GetGroupInviteLink returns the invite CODE for a group.
func (d *Directory) GetGroupInviteLink(ctx context.Context, txtID string, groupJID domain.JID) (string, error) {
	pageJID, err := adapter.ToPageJID(groupJID)
	if err != nil {
		return "", err
	}
	inv, err := d.inviter(ctx, txtID)
	if err != nil {
		return "", err
	}
	code, err := inv.InviteCode(ctx, pageJID, inviteLabel)
	if err != nil {
		return "", err
	}
	// Link() é método e não campo, de propósito: a capability quer que a forma
	// PERIGOSA — o url que qualquer um pode seguir — seja pedida explicitamente,
	// e que ela nunca divirja do código. Este port chama-se GetGroupInviteLink,
	// então pedi-la é exatamente o que ele promete.
	return code.Link(), nil
}

// GetGroupInfoFromLink reads a group BEHIND a link, without joining it.
//
// H89 provou isto ao vivo, reportando `approval=true` no grupo armado — e a
// distinção importa: seguir o link produziria uma solicitação em vez de uma
// entrada, e um chamador que não soubesse disso pediria acesso sem querer.
func (d *Directory) GetGroupInfoFromLink(ctx context.Context, txtID, code string) (any, error) {
	if code == "" {
		return nil, fmt.Errorf("waheadless: empty invite code")
	}
	inv, err := d.inviter(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return inv.InviteInfo(ctx, code, inviteLabel)
}

// grupos filtra a lista de conversas. É onde vive a única regra desta metade.
func (d *Directory) grupos(ctx context.Context, txtID string) ([]waheadless.Chat, error) {
	l, err := d.lister(ctx, txtID)
	if err != nil {
		return nil, err
	}
	lista, err := l.List(ctx, semLimite, listLabel)
	if err != nil {
		return nil, err
	}
	out := make([]waheadless.Chat, 0, len(lista.Chats))
	for _, c := range lista.Chats {
		if c.IsGroup {
			out = append(out, c)
		}
	}
	return out, nil
}

func (d *Directory) lister(ctx context.Context, txtID string) (lister, error) {
	if d.newLister != nil {
		return d.newLister(ctx, txtID)
	}
	eval, err := d.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewChatLister(d.sessions.Runner(), eval), nil
}

func (d *Directory) inviter(ctx context.Context, txtID string) (inviter, error) {
	if d.newInviter != nil {
		return d.newInviter(ctx, txtID)
	}
	eval, err := d.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewGroupManager(d.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.GroupDirectory = (*Directory)(nil)
