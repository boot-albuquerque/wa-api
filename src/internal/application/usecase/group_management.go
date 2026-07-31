package usecase

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	appport "wa-api/internal/application/contracts"
	wm "wa-api/internal/infra/whatsmeow"
)

// GroupManagementUseCase bundles group write operations (create, join, leave,
// settings) that share the same clientProvider dependency.
type GroupManagementUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

func NewGroupManagementUseCase(cp appport.ClientProvider, l appport.Logger) *GroupManagementUseCase {
	return &GroupManagementUseCase{clientProvider: cp, logger: l}
}

func (uc *GroupManagementUseCase) client(ctx context.Context, txtID string) (*whatsmeow.Client, error) {
	c, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil || c == nil {
		return nil, fmt.Errorf("no session")
	}
	return c, nil
}

func (uc *GroupManagementUseCase) parseJID(s string) (types.JID, error) {
	jid, ok := wm.ParseJID(s)
	if !ok {
		return types.EmptyJID, fmt.Errorf("could not parse JID: %s", s)
	}
	return jid, nil
}

// CreateGroup creates a new WhatsApp group.
func (uc *GroupManagementUseCase) CreateGroup(ctx context.Context, txtID string, name string, phones []string) (interface{}, error) {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return nil, err
	}
	jids := make([]types.JID, len(phones))
	for i, p := range phones {
		j, e := uc.parseJID(p)
		if e != nil {
			return nil, e
		}
		jids[i] = j
	}
	return c.CreateGroup(ctx, whatsmeow.ReqCreateGroup{Name: name, Participants: jids})
}

// JoinGroup joins a group via invitation link.
func (uc *GroupManagementUseCase) JoinGroup(ctx context.Context, txtID, code string) (interface{}, error) {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return c.JoinGroupWithLink(context.Background(), code)
}

// LeaveGroup leaves a group.
func (uc *GroupManagementUseCase) LeaveGroup(ctx context.Context, txtID, groupJID string) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
	}
	return c.LeaveGroup(context.Background(), jid)
}

// SetGroupName renames a group.
func (uc *GroupManagementUseCase) SetGroupName(ctx context.Context, txtID, groupJID, name string) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
	}
	return c.SetGroupName(context.Background(), jid, name)
}

// SetGroupTopic sets the group description.
func (uc *GroupManagementUseCase) SetGroupTopic(ctx context.Context, txtID, groupJID, topic string) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
	}
	return c.SetGroupTopic(context.Background(), jid, "", "", topic)
}

// SetGroupPhoto sets the group photo.
func (uc *GroupManagementUseCase) SetGroupPhoto(ctx context.Context, txtID, groupJID string, photoData []byte) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
	}
	_, err = c.SetGroupPhoto(context.Background(), jid, photoData)
	return err
}

// RemoveGroupPhoto removes the group photo.
func (uc *GroupManagementUseCase) RemoveGroupPhoto(ctx context.Context, txtID, groupJID string) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
	}
	_, err = c.SetGroupPhoto(context.Background(), jid, nil)
	return err
}

// SetGroupAnnounce sets announcement-only mode.
func (uc *GroupManagementUseCase) SetGroupAnnounce(ctx context.Context, txtID, groupJID string, announce bool) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
	}
	return c.SetGroupAnnounce(context.Background(), jid, announce)
}

// SetGroupLocked locks/unlocks group settings.
func (uc *GroupManagementUseCase) SetGroupLocked(ctx context.Context, txtID, groupJID string, locked bool) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
	}
	return c.SetGroupLocked(context.Background(), jid, locked)
}

// SetDisappearingTimer sets the disappearing message timer.
func (uc *GroupManagementUseCase) SetDisappearingTimer(ctx context.Context, txtID, groupJID, duration string) error {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return e
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
	return c.SetDisappearingTimer(context.Background(), jid, d, time.Now())
}

// UpdateGroupParticipants adds or removes participants from a group.
func (uc *GroupManagementUseCase) UpdateGroupParticipants(ctx context.Context, txtID, groupJID, action string, phones []string) (interface{}, error) {
	c, err := uc.client(ctx, txtID)
	if err != nil {
		return nil, err
	}
	jid, e := uc.parseJID(groupJID)
	if e != nil {
		return nil, e
	}
	jids := make([]types.JID, len(phones))
	for i, p := range phones {
		j, e := uc.parseJID(p)
		if e != nil {
			return nil, e
		}
		jids[i] = j
	}
	var result []types.GroupParticipant
	if action == "add" {
		result, err = c.UpdateGroupParticipants(ctx, jid, jids, whatsmeow.ParticipantChangeAdd)
	} else {
		result, err = c.UpdateGroupParticipants(ctx, jid, jids, whatsmeow.ParticipantChangeRemove)
	}
	return result, err
}
