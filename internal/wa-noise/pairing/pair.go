// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package pairing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/security/paircrypto"
	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// HandleDeviceNode responde ao <iq> de pair-device e emite o events.QR com os
// codigos que o servidor mandou. Era Client.handlePairDevice.
func HandleDeviceNode(ctx context.Context, t Transport, node *waBinary.Node) {
	pairDevice := node.GetChildByTag("pair-device")
	err := t.SendNode(ctx, waBinary.Node{
		Tag: "iq",
		Attrs: waBinary.Attrs{
			"to":   node.Attrs["from"],
			"id":   node.Attrs["id"],
			"type": "result",
		},
	})
	if err != nil {
		t.Log().Warnf("Failed to send acknowledgement for pair-device request: %v", err)
	}

	evt := &events.QR{Codes: make([]string, 0, len(pairDevice.GetChildren()))}
	for i, child := range pairDevice.GetChildren() {
		if child.Tag != "ref" {
			t.Log().Warnf("pair-device node contains unexpected child tag %s at index %d", child.Tag, i)
			continue
		}
		content, ok := child.Content.([]byte)
		if !ok {
			t.Log().Warnf("pair-device node contains unexpected child content type %T at index %d", child, i)
			continue
		}
		// DetectClientType fica DENTRO do laco, como no original: ele le'
		// globais de store, e hoista-lo seria mudanca de comportamento.
		evt.Codes = append(evt.Codes, MakeQRData(t, content, DetectClientType(t.ConfiguredClientType())))
	}

	t.DispatchEvent(evt)
}

// HandleSuccessNode le' o <iq> de pair-success e dispara a confirmacao do
// pareamento numa goroutine. Era Client.handlePairSuccess.
func HandleSuccessNode(ctx context.Context, t Transport, node *waBinary.Node) {
	// O `id` vem do servidor. Sem o comma-ok, um <iq> de pair-success sem o
	// atributo (ou com tipo inesperado) causava panic aqui — e este handler
	// roda como nodeHandler numa goroutine sem recover, o que derruba o
	// processo inteiro, nao so' a sessao. Ver PATCHES.md, Fase E lote 4.
	id, ok := node.Attrs["id"].(string)
	if !ok {
		t.Log().Warnf("Ignoring pair-success node without a string id attribute")
		return
	}
	pairSuccess := node.GetChildByTag("pair-success")

	deviceIdentityBytes, _ := pairSuccess.GetChildByTag("device-identity").Content.([]byte)
	businessName, _ := pairSuccess.GetChildByTag("biz").Attrs["name"].(string)
	jid, _ := pairSuccess.GetChildByTag("device").Attrs["jid"].(types.JID)
	lid, _ := pairSuccess.GetChildByTag("device").Attrs["lid"].(types.JID)
	platform, _ := pairSuccess.GetChildByTag("platform").Attrs["name"].(string)
	t.SetServerTimeOffset(int64(node.AttrGetter().UnixTime("t").Sub(time.Now().Round(time.Second))))

	go func() {
		err := Confirm(ctx, t, deviceIdentityBytes, id, businessName, platform, jid, lid)
		if err != nil {
			t.Log().Errorf("Failed to pair device: %v", err)
			t.Disconnect()
			t.DispatchEvent(&events.PairError{ID: jid, LID: lid, BusinessName: businessName, Platform: platform, Error: err})
		} else {
			t.Log().Infof("Successfully paired %s", t.Store().ID)
			go t.SendUnifiedSession()
			t.DispatchEvent(&events.PairSuccess{ID: jid, LID: lid, BusinessName: businessName, Platform: platform})
		}
	}()
}

// Confirm valida o device identity assinado pelo aparelho principal, persiste
// as credenciais e devolve a confirmacao ao servidor. Era Client.handlePair.
func Confirm(
	ctx context.Context,
	t Transport,
	deviceIdentityBytes []byte,
	reqID, businessName, platform string,
	jid, lid types.JID,
) error {
	var deviceIdentityContainer waAdv.ADVSignedDeviceIdentityHMAC
	err := proto.Unmarshal(deviceIdentityBytes, &deviceIdentityContainer)
	if err != nil {
		SendError(ctx, t, reqID, errCodeInternal, errTextInternal)
		return &ProtoError{"failed to parse device identity container in pair success message", err}
	}

	h := hmac.New(sha256.New, t.Store().AdvSecretKey)
	if deviceIdentityContainer.GetAccountType() == waAdv.ADVEncryptionType_HOSTED {
		h.Write(paircrypto.AdvHostedAccountSignaturePrefix)
	}
	h.Write(deviceIdentityContainer.Details)

	if !hmac.Equal(h.Sum(nil), deviceIdentityContainer.HMAC) {
		t.Log().Warnf("Invalid HMAC from pair success message")
		SendError(ctx, t, reqID, errCodeUnauthorized, errTextHMACMismatch)
		return ErrInvalidDeviceIdentityHMAC
	}

	var deviceIdentity waAdv.ADVSignedDeviceIdentity
	err = proto.Unmarshal(deviceIdentityContainer.Details, &deviceIdentity)
	if err != nil {
		SendError(ctx, t, reqID, errCodeInternal, errTextInternal)
		return &ProtoError{"failed to parse signed device identity in pair success message", err}
	}

	var deviceIdentityDetails waAdv.ADVDeviceIdentity
	err = proto.Unmarshal(deviceIdentity.Details, &deviceIdentityDetails)
	if err != nil {
		SendError(ctx, t, reqID, errCodeInternal, errTextInternal)
		return &ProtoError{"failed to parse device identity details in pair success message", err}
	}

	if !paircrypto.VerifyAccountSignature(&deviceIdentity, t.Store().IdentityKey, deviceIdentityDetails.GetDeviceType() == waAdv.ADVEncryptionType_HOSTED) {
		SendError(ctx, t, reqID, errCodeUnauthorized, errTextSignatureMismatch)
		return ErrInvalidDeviceSignature
	}

	deviceIdentity.DeviceSignature = paircrypto.GenerateDeviceSignature(&deviceIdentity, t.Store().IdentityKey)[:]

	if !t.PrePairAllowed(jid, platform, businessName) {
		SendError(ctx, t, reqID, errCodeInternal, errTextInternal)
		return ErrRejectedLocally
	}

	t.Store().Account = proto.Clone(&deviceIdentity).(*waAdv.ADVSignedDeviceIdentity)

	mainDeviceLID := lid
	mainDeviceLID.Device = mainDeviceID
	mainDeviceIdentity := *(*[identityKeyLength]byte)(deviceIdentity.AccountSignatureKey)
	deviceIdentity.AccountSignatureKey = nil

	selfSignedDeviceIdentity, err := proto.Marshal(&deviceIdentity)
	if err != nil {
		SendError(ctx, t, reqID, errCodeInternal, errTextInternal)
		return &ProtoError{"failed to marshal self-signed device identity", err}
	}

	t.Store().ID = &jid
	t.Store().LID = lid
	t.Store().BusinessName = businessName
	t.Store().Platform = platform
	err = t.Store().Save(ctx)
	if err != nil {
		SendError(ctx, t, reqID, errCodeInternal, errTextInternal)
		return &DatabaseError{"failed to save device store", err}
	}
	t.StoreLIDPNMapping(ctx, lid, jid)
	err = t.Store().Identities.PutIdentity(ctx, mainDeviceLID.SignalAddress().String(), mainDeviceIdentity)
	if err != nil {
		_ = t.Store().Delete(ctx)
		SendError(ctx, t, reqID, errCodeInternal, errTextInternal)
		return &DatabaseError{"failed to store main device identity", err}
	}

	// Expect a disconnect after this and don't dispatch the usual Disconnected event
	t.ExpectDisconnect()

	err = t.SendNode(ctx, waBinary.Node{
		Tag: "iq",
		Attrs: waBinary.Attrs{
			"to":   types.ServerJID,
			"type": "result",
			"id":   reqID,
		},
		Content: []waBinary.Node{{
			Tag: "pair-device-sign",
			Content: []waBinary.Node{{
				Tag: "device-identity",
				Attrs: waBinary.Attrs{
					"key-index": deviceIdentityDetails.GetKeyIndex(),
				},
				Content: selfSignedDeviceIdentity,
			}},
		}},
	})
	if err != nil {
		_ = t.Store().Delete(ctx)
		return fmt.Errorf("failed to send pairing confirmation: %w", err)
	}
	return nil
}

// SendError devolve ao servidor o <iq type="error"> que aborta o pareamento.
// Era Client.sendPairError.
func SendError(ctx context.Context, t Transport, id string, code int, text string) {
	err := t.SendNode(ctx, waBinary.Node{
		Tag: "iq",
		Attrs: waBinary.Attrs{
			"to":   types.ServerJID,
			"type": "error",
			"id":   id,
		},
		Content: []waBinary.Node{{
			Tag: "error",
			Attrs: waBinary.Attrs{
				"code": code,
				"text": text,
			},
		}},
	})
	if err != nil {
		t.Log().Errorf("Failed to send pair error node: %v", err)
	}
}
