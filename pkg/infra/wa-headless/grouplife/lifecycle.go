// Package grouplife adapts creating, joining and leaving groups to the
// application's GroupLifecycle port.
package grouplife

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
)

const (
	createLabel = "adapter/create-group"
	joinLabel   = "adapter/join-group"
	leaveLabel  = "adapter/leave-group"
)

// lifecycle is the slice of the page capability this adapter uses.
type lifecycle interface {
	Ensure(ctx context.Context, subject string, participants []string, label string) (waheadless.GroupCreated, error)
	JoinByInvite(ctx context.Context, code, label string) (waheadless.GroupJoined, error)
	Leave(ctx context.Context, groupJID, label string) error
}

// Manager implements appport.GroupLifecycle over a headless session.
type Manager struct {
	sessions *adapter.Sessions
	// newLifecycle is overridable in tests. Nil uses the real page capability.
	newLifecycle func(ctx context.Context, txtID string) (lifecycle, error)
}

// NewManager builds the adapter.
func NewManager(sessions *adapter.Sessions) *Manager { return &Manager{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (m *Manager) EnsureSession(ctx context.Context, txtID string) error {
	return m.sessions.EnsureSession(ctx, txtID)
}

// CreateGroup makes a group with the given name and participants.
//
// A capability chama-se Ensure e não Create, e a diferença chega ao chamador no
// campo Created: um grupo com o mesmo nome que já exista é DEVOLVIDO em vez de
// duplicado. Achatar isso faria duas chamadas iguais parecerem ter criado dois
// grupos quando criaram um.
func (m *Manager) CreateGroup(ctx context.Context, txtID, name string, participants []domain.JID, _ domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("waheadless: group name is empty")
	}
	pageJIDs := make([]string, 0, len(participants))
	for _, p := range participants {
		pageJID, err := adapter.ToPageJID(p)
		if err != nil {
			return nil, err
		}
		pageJIDs = append(pageJIDs, pageJID)
	}

	l, err := m.lifecycle(ctx, txtID)
	if err != nil {
		return nil, err
	}
	grupo, err := l.Ensure(ctx, name, pageJIDs, createLabel)
	if err != nil {
		return nil, err
	}
	// Três campos é o que a página dá sobre o grupo recém-criado, e o resto do
	// domain.GroupInfo fica no zero — a mesma regra do groupdir.
	return &domain.CreatedGroup{
		Group: &domain.GroupInfo{
			JID:              domain.JID(grupo.JID),
			Name:             grupo.Subject,
			ParticipantCount: grupo.Participants,
			Participants:     []domain.GroupParticipant{},
		},
		Created: grupo.Created,
	}, nil
}

// JoinGroup follows an invite code.
//
// O resultado carrega Pending, e é a informação que não pode ser perdida: um
// grupo com aprovação transforma a entrada numa SOLICITAÇÃO, e reportá-la como
// adesão faria o chamador anunciar uma entrada que não aconteceu.
func (m *Manager) JoinGroup(ctx context.Context, txtID, code string) (any, error) {
	if code == "" {
		return nil, fmt.Errorf("waheadless: empty invite code")
	}
	l, err := m.lifecycle(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return l.JoinByInvite(ctx, code, joinLabel)
}

// LeaveGroup leaves a group.
func (m *Manager) LeaveGroup(ctx context.Context, txtID string, groupJID domain.JID) error {
	pageJID, err := adapter.ToPageJID(groupJID)
	if err != nil {
		return err
	}
	l, err := m.lifecycle(ctx, txtID)
	if err != nil {
		return err
	}
	return l.Leave(ctx, pageJID, leaveLabel)
}

func (m *Manager) lifecycle(ctx context.Context, txtID string) (lifecycle, error) {
	if m.newLifecycle != nil {
		return m.newLifecycle(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewGroupManager(m.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.GroupLifecycle = (*Manager)(nil)
