// Package groupmembers adapts changing group membership to the application's
// port, carrying the CONFIRMATION status instead of flattening it.
//
// # Por que este adaptador existe com uma forma diferente dos outros
//
// A invariante 14 exige que nenhuma escrita devolva sucesso silencioso. Aqui ela
// é impossível de cumprir da forma habitual, e isso foi MEDIDO: H58 para
// adicionar e remover, H65 para promover e rebaixar. A mudança CHEGA ao
// servidor, e a sessão que agiu não a vê — a metadata não refresca e nenhum
// aviso de sistema chega. A confirmação só aparece em OUTRA sessão.
//
// A capability já sabia disso e já dizia, em `Membership.Verified`:
//
//	"Verified diz se a mudança foi CONFIRMADA. Neste build é verdadeiro só para
//	um no-op, porque uma mudança real é invisível para a sessão que a fez.
//	Reportar uma mudança como confirmada aqui seria uma mentira que o chamador
//	não pode detetar, e uma que já custou a este repositório um grupo de
//	laboratório quebrado."
//
// O que faltava era um lugar no PORT para dizê-lo. A decisão 86 criou-o
// (domain.ParticipantsUpdate), e o trabalho deste adaptador é carregar a verdade
// da capability até lá — em vez de a achatar num `any` que ninguém inspeciona.
package groupmembers

import (
	"context"
	"fmt"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
)

const membersLabel = "adapter/update-group-participants"

// naoObservavel é o motivo que acompanha toda mudança REAL neste transporte.
//
// É constante porque a causa é sempre a mesma e está medida: escrevê-la à mão em
// cada caminho convidaria a versões que divergem, e a divergência faria parecer
// que há mais de uma causa.
const naoObservavel = "a mudança foi enviada e esta sessão não a observa: " +
	"neste build a metadata não refresca e nenhum aviso de sistema chega " +
	"(medido em H58 e H65); a confirmação só aparece noutra sessão"

// members is the slice of the page capability this adapter uses.
type members interface {
	AddParticipant(ctx context.Context, groupJID, participantJID, label string) (headless.GroupMembership, error)
	RemoveParticipant(ctx context.Context, groupJID, participantJID, label string) (headless.GroupMembership, error)
}

// Manager implements the participant half of appport.GroupSettings.
type Manager struct {
	sessions *adapter.Sessions
	// newMembers is overridable in tests. Nil uses the real page capability.
	newMembers func(ctx context.Context, txtID string) (members, error)
}

// NewManager builds the adapter.
func NewManager(sessions *adapter.Sessions) *Manager { return &Manager{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (m *Manager) EnsureSession(ctx context.Context, txtID string) error {
	return m.sessions.EnsureSession(ctx, txtID)
}

// UpdateGroupParticipants adds or removes participants, and reports whether this
// session could CONFIRM what it did.
//
// Confirmed is true only when every change was a no-op — already a member, or
// already not one — because that is the only case this session can verify. A
// real change returns Confirmed false WITH the measured reason, which is the
// whole point of decision 86: the uncertainty is declared, not swallowed.
func (m *Manager) UpdateGroupParticipants(ctx context.Context, txtID string, groupJID domain.JID, participants []domain.JID, action domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
	if action != domain.ParticipantAdd && action != domain.ParticipantRemove {
		return domain.ParticipantsUpdate{}, fmt.Errorf("headless: unknown participant action %q", action)
	}
	if len(participants) == 0 {
		return domain.ParticipantsUpdate{}, fmt.Errorf("headless: no participants to update")
	}

	pageGroup, err := adapter.ToPageJID(groupJID)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}

	mem, err := m.members(ctx, txtID)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}

	// todosNoOp começa verdadeiro e só desce: uma ÚNICA mudança real torna o
	// lote inteiro não confirmável, porque a sessão não observa nenhuma delas.
	todosNoOp := true

	for _, p := range participants {
		pageJID, convErr := adapter.ToPageJID(p)
		if convErr != nil {
			return domain.ParticipantsUpdate{}, convErr
		}

		var r headless.GroupMembership
		if action == domain.ParticipantAdd {
			r, err = mem.AddParticipant(ctx, pageGroup, pageJID, membersLabel)
		} else {
			r, err = mem.RemoveParticipant(ctx, pageGroup, pageJID, membersLabel)
		}
		if err != nil {
			// Aborta em vez de continuar: um lote parcialmente aplicado cuja
			// falha fosse engolida deixaria o grupo num estado que ninguém pediu
			// e que o chamador não saberia interrogar.
			return domain.ParticipantsUpdate{}, err
		}
		if !r.Verified {
			todosNoOp = false
		}
	}

	// Participants fica VAZIO, e não é um palpite: este motor não lê nenhum
	// roster de volta — o que ele mediu foram CONTAGENS por participante
	// (antes, depois-desejado, no-op), e servi-las como participantes seria
	// inventar identidades que ninguém observou. O que ele sabe dizer é se
	// confirmou, e isso vai em Confirmed/Reason.
	out := domain.ParticipantsUpdate{Confirmed: todosNoOp}
	if !out.Confirmed {
		out.Reason = naoObservavel
	}
	// A invariante do tipo é verificada AQUI, e não confiada: um caminho novo
	// que esqueça o motivo falha na origem, não no consumidor.
	if err := out.Valida(); err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	return out, nil
}

func (m *Manager) members(ctx context.Context, txtID string) (members, error) {
	if m.newMembers != nil {
		return m.newMembers(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return headless.NewGroupManager(m.sessions.Runner(), eval), nil
}

// Prova em tempo de compilacao de que este adaptador satisfaz o port.
//
// Ate a decisao 92 esta linha NAO EXISTIA, e a ausencia estava documentada:
// appport.GroupSettings pedia tambem nome, topico, foto, anuncio, trava e
// temporizador, e declarar que este adaptador o satisfazia seria a mentira que
// a decisao 80 existe para impedir. O comentario que dizia isso descrevia uma
// costura real no codigo — dois adaptadores, um port, nenhum capaz de provar
// nada. A decisao 92 cortou o port onde a costura ja estava.
var _ appport.GroupParticipants = (*Manager)(nil)
