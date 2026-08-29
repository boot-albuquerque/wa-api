// Package groupset adapts the application's GroupInfoSettings port: the
// settings a group keeps about ITSELF.
//
// O que esta fatia carrega não é o mapeamento — é que quatro escritas que
// parecem a mesma operação repetida têm TRÊS confirmações diferentes, e cada
// uma tem de ser lida no seu próprio vocabulário.
package groupset

import (
	"context"
	"fmt"
	"strings"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
)

const (
	nameLabel     = "adapter/set-group-name"
	topicLabel    = "adapter/set-group-topic"
	announceLabel = "adapter/set-group-announce"
	lockedLabel   = "adapter/set-group-locked"
)

// settings is the slice of the group capability this adapter uses.
type settings interface {
	SetSubject(ctx context.Context, groupJID, subject, label string) (headless.GroupRename, error)
	SetDescription(ctx context.Context, groupJID, description, label string) (headless.GroupDescribed, error)
	SetPolicy(ctx context.Context, groupJID string, p headless.GroupPolicy, on bool, label string) (headless.GroupPolicyChange, error)
}

// Manager implements appport.GroupInfoSettings over a headless session.
//
// A porta é estreita porque a decisão 92 a estreitou, e esta é a prova de que
// a divisão era real: ANTES dela, este adaptador e o groupmembers implementavam
// pedaços de uma porta larga e NENHUM dos dois podia declarar que a satisfazia.
// Agora cada um declara a sua, em tempo de compilação.
type Manager struct {
	sessions *adapter.Sessions
	// newSettings is overridable in tests. Nil uses the real page capability.
	newSettings func(ctx context.Context, txtID string) (settings, error)
}

// NewManager builds the adapter.
func NewManager(sessions *adapter.Sessions) *Manager { return &Manager{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (m *Manager) EnsureSession(ctx context.Context, txtID string) error {
	return m.sessions.EnsureSession(ctx, txtID)
}

// SetGroupName renames a group.
func (m *Manager) SetGroupName(ctx context.Context, txtID string, group domain.JID, name string) error {
	s, pageJID, err := m.prepare(ctx, txtID, group)
	if err != nil {
		return err
	}
	out, err := s.SetSubject(ctx, pageJID, name, nameLabel)
	if err != nil {
		return err
	}
	return renameUnconfirmed(out, name)
}

// SetGroupTopic changes the group description.
func (m *Manager) SetGroupTopic(ctx context.Context, txtID string, group domain.JID, topic string) error {
	s, pageJID, err := m.prepare(ctx, txtID, group)
	if err != nil {
		return err
	}
	out, err := s.SetDescription(ctx, pageJID, topic, topicLabel)
	if err != nil {
		return err
	}
	return describeUnconfirmed(out, topic)
}

// SetGroupAnnounce restricts sending to admins.
func (m *Manager) SetGroupAnnounce(ctx context.Context, txtID string, group domain.JID, announce bool) error {
	return m.policy(ctx, txtID, group, headless.PolicyMessagesAdminsOnly, announce, announceLabel)
}

// SetGroupLocked restricts editing the group's own information to admins.
func (m *Manager) SetGroupLocked(ctx context.Context, txtID string, group domain.JID, locked bool) error {
	return m.policy(ctx, txtID, group, headless.PolicyInfoAdminsOnly, locked, lockedLabel)
}

// ---- as três confirmações, cada uma no seu vocabulário ----
//
// A invariante 14 diz que toda escrita relê a sua pós-condição. Este port
// devolve só `error`, então o que sobra de cada confirmação é a RECUSA. O que
// não se pode fazer é aplicar a mesma regra às três: a página confirma cada uma
// de um jeito, e isso foi medido, não escolhido.

// renameUnconfirmed refuses a rename the page could not point at.
//
// Rename não tem booleano de verificação: tem `Field`, o NOME do campo do
// modelo que carregou a mudança — "uma capability que não sabe dizer onde olhou
// não pode ser conferida pelo próximo leitor". Campo vazio é isso: não soube
// dizer. AlreadyInState é o no-op, sem valor novo a confirmar.
func renameUnconfirmed(r headless.GroupRename, want string) error {
	if r.AlreadyInState || r.Field != "" {
		return nil
	}
	return fmt.Errorf("headless: nome pedido (%d bytes) sem campo do modelo que o confirme", len(want))
}

// describeUnconfirmed refuses a description the server read back DIFFERENT.
//
// Described.Text é o texto RELIDO depois da mudança, então aqui a pós-condição
// é comparável — a mais forte das três. A comparação apara os extremos, porque
// o servidor normaliza espaço e uma diferença só nisso seria falso negativo.
//
// O texto NÃO entra na mensagem: descrição de grupo é conteúdo do usuário, e
// mensagem de erro é registro. Só comprimentos e o nome do campo.
func describeUnconfirmed(d headless.GroupDescribed, want string) error {
	if strings.TrimSpace(d.Text) == strings.TrimSpace(want) {
		return nil
	}
	return fmt.Errorf("headless: descricao pedida (%d bytes) relida diferente (%d bytes, campo %q)",
		len(want), len(d.Text), d.Source)
}

// policyUnconfirmed refuses a policy change the page did not read back.
func policyUnconfirmed(c headless.GroupPolicyChange) error {
	if c.NoOp || c.Verified {
		return nil
	}
	return fmt.Errorf("headless: politica %q pedida (%t) sem confirmacao lida de volta", c.Policy, c.Wanted)
}

func (m *Manager) policy(ctx context.Context, txtID string, group domain.JID, p headless.GroupPolicy, on bool, label string) error {
	s, pageJID, err := m.prepare(ctx, txtID, group)
	if err != nil {
		return err
	}
	change, err := s.SetPolicy(ctx, pageJID, p, on, label)
	if err != nil {
		return err
	}
	return policyUnconfirmed(change)
}

// prepare converts the identity and resolves the capability, in that order: a
// jid the page would refuse must not consume a session slot.
func (m *Manager) prepare(ctx context.Context, txtID string, group domain.JID) (settings, string, error) {
	pageJID, err := adapter.ToPageJID(group)
	if err != nil {
		return nil, "", err
	}
	s, err := m.capability(ctx, txtID)
	if err != nil {
		return nil, "", err
	}
	return s, pageJID, nil
}

func (m *Manager) capability(ctx context.Context, txtID string) (settings, error) {
	if m.newSettings != nil {
		return m.newSettings(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return headless.NewGroupManager(m.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port. Antes da decisão 92
// esta linha não existia porque não podia existir.
var _ appport.GroupInfoSettings = (*Manager)(nil)
