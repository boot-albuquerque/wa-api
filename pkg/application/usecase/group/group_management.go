package group

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GroupManagementUseCase bundles group write operations (create, join, leave,
// settings) that share the same session dependency.
type GroupManagementUseCase struct {
	lifecycle appport.GroupLifecycle
	settings  appport.GroupSettings
	jids      appport.JIDResolver
	logger    appport.Logger
}

func NewGroupManagementUseCase(gl appport.GroupLifecycle, gs appport.GroupSettings, jr appport.JIDResolver, l appport.Logger) *GroupManagementUseCase {
	return &GroupManagementUseCase{lifecycle: gl, settings: gs, jids: jr, logger: l}
}

// ensure mantém a guarda de sessão que o antigo helper client() aplicava
// antes de cada operação, com a mesma mensagem de erro que os handlers já
// recebiam.
func (uc *GroupManagementUseCase) ensure(ctx context.Context, txtID string) error {
	if err := uc.settings.EnsureSession(ctx, txtID); err != nil {
		return fmt.Errorf("no session")
	}
	return nil
}

func (uc *GroupManagementUseCase) parseJID(ctx context.Context, s string) (domain.JID, error) {
	jid, err := uc.jids.ResolveJID(ctx, s)
	if err != nil {
		return "", fmt.Errorf("could not parse JID: %s", s)
	}
	return jid, nil
}

func (uc *GroupManagementUseCase) parseJIDs(ctx context.Context, in []string) ([]domain.JID, error) {
	out := make([]domain.JID, len(in))
	for i, p := range in {
		j, err := uc.parseJID(ctx, p)
		if err != nil {
			return nil, err
		}
		out[i] = j
	}
	return out, nil
}

// CreateGroup creates a new WhatsApp group.
func (uc *GroupManagementUseCase) CreateGroup(ctx context.Context, txtID string, name string, phones []string) (interface{}, error) {
	if err := uc.ensure(ctx, txtID); err != nil {
		return nil, err
	}
	jids, err := uc.parseJIDs(ctx, phones)
	if err != nil {
		return nil, err
	}
	return uc.lifecycle.CreateGroup(ctx, txtID, name, jids)
}

// JoinGroup joins a group via invitation link.
func (uc *GroupManagementUseCase) JoinGroup(ctx context.Context, txtID, code string) (interface{}, error) {
	if err := uc.ensure(ctx, txtID); err != nil {
		return nil, err
	}
	return uc.lifecycle.JoinGroup(ctx, txtID, code)
}

// LeaveGroup leaves a group.
func (uc *GroupManagementUseCase) LeaveGroup(ctx context.Context, txtID, groupJID string) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	return uc.lifecycle.LeaveGroup(ctx, txtID, jid)
}

// SetGroupName renames a group.
func (uc *GroupManagementUseCase) SetGroupName(ctx context.Context, txtID, groupJID, name string) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	return uc.settings.SetGroupName(ctx, txtID, jid, name)
}

// SetGroupTopic sets the group description.
func (uc *GroupManagementUseCase) SetGroupTopic(ctx context.Context, txtID, groupJID, topic string) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	return uc.settings.SetGroupTopic(ctx, txtID, jid, topic)
}

// SetGroupPhoto sets the group photo.
func (uc *GroupManagementUseCase) SetGroupPhoto(ctx context.Context, txtID, groupJID string, photoData []byte) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	return uc.settings.SetGroupPhoto(ctx, txtID, jid, photoData)
}

// RemoveGroupPhoto removes the group photo.
func (uc *GroupManagementUseCase) RemoveGroupPhoto(ctx context.Context, txtID, groupJID string) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	return uc.settings.SetGroupPhoto(ctx, txtID, jid, nil)
}

// SetGroupAnnounce sets announcement-only mode.
func (uc *GroupManagementUseCase) SetGroupAnnounce(ctx context.Context, txtID, groupJID string, announce bool) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	return uc.settings.SetGroupAnnounce(ctx, txtID, jid, announce)
}

// SetGroupLocked locks/unlocks group settings.
func (uc *GroupManagementUseCase) SetGroupLocked(ctx context.Context, txtID, groupJID string, locked bool) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	return uc.settings.SetGroupLocked(ctx, txtID, jid, locked)
}

// SetDisappearingTimer sets the disappearing message timer.
func (uc *GroupManagementUseCase) SetDisappearingTimer(ctx context.Context, txtID, groupJID, duration string) error {
	if err := uc.ensure(ctx, txtID); err != nil {
		return err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return err
	}
	var d time.Duration
	switch duration {
	case "24h":
		d = 24 * time.Hour
	case "7d":
		d = 7 * 24 * time.Hour
	case "90d":
		d = 90 * 24 * time.Hour
	default:
		d = 0
	}
	return uc.settings.SetDisappearingTimer(ctx, txtID, jid, d, time.Now())
}

// UpdateGroupParticipants adds or removes participants from a group.
func (uc *GroupManagementUseCase) UpdateGroupParticipants(ctx context.Context, txtID, groupJID, action string, phones []string) (interface{}, error) {
	if err := uc.ensure(ctx, txtID); err != nil {
		return nil, err
	}
	jid, err := uc.parseJID(ctx, groupJID)
	if err != nil {
		return nil, err
	}
	jids, err := uc.parseJIDs(ctx, phones)
	if err != nil {
		return nil, err
	}

	// Qualquer ação diferente de "add" é remoção — regra preservada do
	// upstream, que não validava o valor recebido.
	participantAction := domain.ParticipantRemove
	if action == "add" {
		participantAction = domain.ParticipantAdd
	}
	return uc.settings.UpdateGroupParticipants(ctx, txtID, jid, jids, participantAction)
}
