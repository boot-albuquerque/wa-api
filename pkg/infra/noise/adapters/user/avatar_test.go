package user

import (
	"context"
	"errors"
	"testing"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"

	"wa-api/pkg/domain"

	"wa-api/internal/noise"
	"wa-api/internal/noise/protocol/types"
)

// TestUserAdapter_GetProfilePicture_NoSession.
func TestUserAdapter_GetProfilePicture_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, err := a.GetProfilePicture(context.Background(), "u1", "x@y.com", false)
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetProfilePicture code = %q", testkit.AppErrCode(err))
	}
}

// TestUserAdapter_GetProfilePicture_NilPictureInfo — o branch pic==nil,err==nil
// do wa-noise (ExistingID/If-Modified-Since; nunca ocorre hoje na prática já
// que ExistingID é sempre "") também devolve domain.ErrAvatarNotFound, pela
// mesma razão do caso NotSet: nada pra mostrar, não é falha.
func TestUserAdapter_GetProfilePicture_NilPictureInfo(t *testing.T) {
	fake := &testkit.Fake{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *noise.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return nil, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	got, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if !errors.Is(err, domain.ErrAvatarNotFound) {
		t.Fatalf("GetProfilePicture err = %v, want domain.ErrAvatarNotFound", err)
	}
	if got != nil {
		t.Errorf("GetProfilePicture = %v, want nil", got)
	}
}

// TestUserAdapter_GetProfilePicture_OK mapeia ID e URL.
func TestUserAdapter_GetProfilePicture_OK(t *testing.T) {
	fake := &testkit.Fake{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *noise.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return &types.ProfilePictureInfo{ID: "abc", URL: "https://example/pic"}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	got, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if err != nil {
		t.Fatalf("GetProfilePicture = %v", err)
	}
	if got.ID != "abc" || got.URL != "https://example/pic" {
		t.Errorf("GetProfilePicture = %+v", got)
	}
}

// TestUserAdapter_GetProfilePicture_NotSet — ErrProfilePictureNotSet (o
// contato genuinamente não tem foto) devolve (nil, domain.ErrAvatarNotFound):
// era indistinguível de falha real antes desta correção, subia como 500 no
// handler HTTP para o caso mais comum (maioria dos contatos sem foto).
func TestUserAdapter_GetProfilePicture_NotSet(t *testing.T) {
	fake := &testkit.Fake{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *noise.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return nil, noise.ErrProfilePictureNotSet
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	got, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if !errors.Is(err, domain.ErrAvatarNotFound) {
		t.Fatalf("GetProfilePicture err = %v, want domain.ErrAvatarNotFound", err)
	}
	if got != nil {
		t.Errorf("GetProfilePicture = %v, want nil", got)
	}
}

// TestUserAdapter_GetProfilePicture_Unauthorized — ErrProfilePictureUnauthorized
// (contato TEM foto mas escondeu-a via privacidade) devolve (nil,
// domain.ErrAvatarUnauthorized) — distinto de ErrAvatarNotFound, pelo mesmo
// motivo: não é falha de servidor, mas também não é "nunca vai ter foto".
func TestUserAdapter_GetProfilePicture_Unauthorized(t *testing.T) {
	fake := &testkit.Fake{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *noise.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return nil, noise.ErrProfilePictureUnauthorized
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	got, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if !errors.Is(err, domain.ErrAvatarUnauthorized) {
		t.Fatalf("GetProfilePicture err = %v, want domain.ErrAvatarUnauthorized", err)
	}
	if got != nil {
		t.Errorf("GetProfilePicture = %v, want nil", got)
	}
}

// TestUserAdapter_GetProfilePicture_PropagatesError.
func TestUserAdapter_GetProfilePicture_PropagatesError(t *testing.T) {
	sdkErr := errors.New("not found")
	fake := &testkit.Fake{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *noise.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	_, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if err == nil {
		t.Fatal("GetProfilePicture não propagou erro")
	}
}
