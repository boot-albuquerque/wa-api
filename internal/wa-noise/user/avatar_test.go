// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"encoding/base64"
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// pictureResponse monta a resposta de um <iq> de foto de perfil.
func pictureResponse(attrs waBinary.Attrs) *waBinary.Node {
	return &waBinary.Node{
		Tag:     "iq",
		Content: []waBinary.Node{{Tag: pictureNodeTag, Attrs: attrs}},
	}
}

func TestGetProfilePictureInfoReadsEveryField(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	hash := []byte{1, 2, 3}
	f.resp = []*waBinary.Node{pictureResponse(waBinary.Attrs{
		"id": "pic-1", "url": "https://x/y", "type": profilePictureTypeImage,
		"direct_path": "/v/t", "hash": base64.StdEncoding.EncodeToString(hash),
	})}

	got, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "pic-1" || got.URL != "https://x/y" || got.DirectPath != "/v/t" {
		t.Errorf("got %+v", got)
	}
	if string(got.Hash) != string(hash) {
		t.Errorf("hash = %v", got.Hash)
	}
	// params nil equivale a GetProfilePictureParams{}: imagem cheia, para o
	// servidor, com o alvo no campo Target.
	iq := f.sent[0]
	if iq.Namespace != profilePictureIQNamespace || iq.To != types.ServerJID || iq.Target != userTestPNJID {
		t.Errorf("envelope = %+v", iq)
	}
	picture := iq.Content.([]waBinary.Node)[0]
	if picture.Attrs["type"] != profilePictureTypeImage || picture.Attrs["query"] != profilePictureQueryURL {
		t.Errorf("attrs = %v", picture.Attrs)
	}
}

func TestGetProfilePictureInfoPreviewAndOptionalAttrs(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	f.resp = []*waBinary.Node{pictureResponse(waBinary.Attrs{"id": "p", "url": "u", "type": "preview", "direct_path": "d"})}

	commonGID := types.NewJID("55511", types.GroupServer)
	_, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, &GetProfilePictureParams{
		Preview:    true,
		ExistingID: "old-id",
		InviteCode: "inv",
		CommonGID:  commonGID,
		PersonaID:  "persona",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := f.sent[0].Content.([]waBinary.Node)[0].Attrs
	if attrs["type"] != profilePictureTypePreview || attrs["id"] != "old-id" ||
		attrs["invite"] != "inv" || attrs["common_gid"] != commonGID || attrs["persona_id"] != "persona" {
		t.Errorf("attrs = %v", attrs)
	}
}

// Privacy token presente vai como <tctoken> dentro do <picture>.
func TestGetProfilePictureInfoAttachesPrivacyToken(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.token = &store.PrivacyToken{Token: []byte("tok")}
	f.resp = []*waBinary.Node{pictureResponse(waBinary.Attrs{"id": "p", "url": "u", "type": "image", "direct_path": "d"})}

	if _, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := f.sent[0].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)
	if len(content) != 1 || content[0].Tag != profilePictureTokenNodeTag {
		t.Fatalf("content = %v", content)
	}
	if string(content[0].Content.([]byte)) != "tok" {
		t.Errorf("token = %v", content[0].Content)
	}
}

// Foto de comunidade sai pelo namespace de grupo, com a resposta embrulhada em
// <pictures>. E' o unico caminho deste dominio que usa group.IQNamespace.
func TestGetProfilePictureInfoCommunityUsesGroupNamespace(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	f.resp = []*waBinary.Node{{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag: picturesNodeTag,
			Content: []waBinary.Node{{
				Tag:   pictureNodeTag,
				Attrs: waBinary.Attrs{"id": "p", "url": "u", "type": "image", "direct_path": "d"},
			}},
		}},
	}}

	communityJID := types.NewJID("55511", types.GroupServer)
	got, err := GetProfilePictureInfo(t.Context(), f, communityJID, &GetProfilePictureParams{IsCommunity: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "p" {
		t.Errorf("got %+v", got)
	}
	iq := f.sent[0]
	if iq.Namespace != group.IQNamespace {
		t.Errorf("namespace = %q, queria %q", iq.Namespace, group.IQNamespace)
	}
	if iq.To != communityJID || iq.Target != types.EmptyJID {
		t.Errorf("to = %s, target = %s", iq.To, iq.Target)
	}
	pictures := iq.Content.([]waBinary.Node)[0]
	if pictures.Tag != picturesNodeTag {
		t.Fatalf("tag = %q", pictures.Tag)
	}
	inner := pictures.Content.([]waBinary.Node)[0]
	if inner.Attrs["parent_group_jid"] != communityJID {
		t.Errorf("attrs = %v", inner.Attrs)
	}
}

func TestGetProfilePictureInfoCommunityMissingWrapperIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, &GetProfilePictureParams{IsCommunity: true})
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != picturesNodeTag {
		t.Errorf("got %v", err)
	}
}

// Os dois erros de IQ que viram erro de dominio, preservando o embrulho.
func TestGetProfilePictureInfoMapsIQErrors(t *testing.T) {
	cases := []struct {
		name string
		iq   error
		want error
	}{
		{"401 vira unauthorized", testIQErrors.NotAuthorized, ErrProfilePictureUnauthorized},
		{"404 vira not set", testIQErrors.NotFound, ErrProfilePictureNotSet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTransport()
			f.withStores()
			f.err = []error{tc.iq}
			_, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			// O erro de IQ original continua alcancavel por Unwrap.
			if !errors.Is(err, tc.iq) {
				t.Errorf("o erro de IQ original se perdeu: %v", err)
			}
		})
	}
}

func TestGetProfilePictureInfoPropagatesOtherErrors(t *testing.T) {
	boom := errors.New("socket morreu")
	f := newFakeTransport()
	f.withStores()
	f.err = []error{boom}
	_, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil)
	if !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

// Com ExistingID e sem <picture> na resposta, a foto nao mudou: (nil, nil).
// Sem ExistingID, a ausencia e' erro de esquema.
func TestGetProfilePictureInfoMissingPictureNode(t *testing.T) {
	t.Run("com ExistingID e' nao-modificado", func(t *testing.T) {
		f := newFakeTransport()
		f.withStores()
		f.resp = []*waBinary.Node{{Tag: "iq"}}
		got, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, &GetProfilePictureParams{ExistingID: "old"})
		if err != nil || got != nil {
			t.Errorf("got (%v, %v), want (nil, nil)", got, err)
		}
	})
	t.Run("sem ExistingID e' erro", func(t *testing.T) {
		f := newFakeTransport()
		f.withStores()
		f.resp = []*waBinary.Node{{Tag: "iq"}}
		_, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil)
		var missing *testElementMissing
		if !errors.As(err, &missing) || missing.Tag != pictureNodeTag {
			t.Errorf("got %v", err)
		}
	})
}

// Os dois status que o servidor devolve DENTRO do <picture>, em vez de como
// erro de IQ.
func TestGetProfilePictureInfoInlineStatuses(t *testing.T) {
	t.Run("304 nao modificado", func(t *testing.T) {
		f := newFakeTransport()
		f.withStores()
		f.resp = []*waBinary.Node{pictureResponse(waBinary.Attrs{"status": "304"})}
		got, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil)
		if err != nil || got != nil {
			t.Errorf("got (%v, %v), want (nil, nil)", got, err)
		}
	})
	t.Run("204 sem foto", func(t *testing.T) {
		f := newFakeTransport()
		f.withStores()
		f.resp = []*waBinary.Node{pictureResponse(waBinary.Attrs{"status": "204"})}
		got, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil)
		if !errors.Is(err, ErrProfilePictureNotSet) || got != nil {
			t.Errorf("got (%v, %v)", got, err)
		}
	})
}

// Atributo obrigatorio faltando devolve a info PARCIAL junto do erro — o
// chamador historico depende disso para nao perder o que veio.
func TestGetProfilePictureInfoReturnsPartialInfoOnAttrError(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	f.resp = []*waBinary.Node{pictureResponse(waBinary.Attrs{"id": "pic-1"})}
	got, err := GetProfilePictureInfo(t.Context(), f, userTestPNJID, nil)
	if err == nil {
		t.Fatal("esperava erro de atributo ausente")
	}
	if got == nil || got.ID != "pic-1" {
		t.Errorf("a info parcial deveria voltar mesmo com erro: %+v", got)
	}
}
