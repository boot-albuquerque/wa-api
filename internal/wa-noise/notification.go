// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"time"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/notification"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

func (cli *Client) handleEncryptNotification(ctx context.Context, node *waBinary.Node) {
	from := node.AttrGetter().JID("from")
	if from == types.ServerJID {
		count := node.GetChildByTag("count")
		ag := count.AttrGetter()
		otksLeft := ag.Int("value")
		if !ag.OK() {
			cli.Log.Warnf("Didn't get number of OTKs left in encryption notification %s", node.XMLString())
			return
		}
		cli.Log.Infof("Got prekey count from server: %s", node.XMLString())
		if otksLeft < MinPreKeyCount {
			cli.uploadPreKeys(ctx, false)
		}
	} else if _, ok := node.GetOptionalChildByTag("identity"); ok {
		cli.Log.Debugf("Got identity change for %s: %s, deleting all identities/sessions for that number", from, node.XMLString())
		err := cli.Store.Identities.DeleteAllIdentities(ctx, from.User)
		if err != nil {
			cli.Log.Warnf("Failed to delete all identities of %s from store after identity change: %v", from, err)
		}
		err = cli.Store.Sessions.DeleteAllSessions(ctx, from.User)
		if err != nil {
			cli.Log.Warnf("Failed to delete all sessions of %s from store after identity change: %v", from, err)
		}
		ts := node.AttrGetter().UnixTime("t")
		storageLID := cli.resolveTCTokenStorageLID(ctx, from)
		pt, err := cli.Store.PrivacyTokens.GetPrivacyToken(ctx, storageLID)
		if err != nil {
			cli.Log.Debugf("Failed to load tctoken for identity change re-issue %s: %v", storageLID, err)
		}
		storedSenderTS := time.Time{}
		if pt != nil {
			storedSenderTS = pt.SenderTimestamp
		}
		if cli.validateAndSetTCTokenSenderTS(storageLID, storedSenderTS) {
			senderTS := cli.getTCTokenSenderTS(storageLID)
			if !senderTS.IsZero() {
				cli.Log.Debugf("Identity changed for %s, re-issuing tctoken", from)
				go cli.issuePrivacyTokenAndSave(storageLID, senderTS)
			}
		}
		cli.dispatchEvent(&events.IdentityChange{JID: from, Timestamp: ts})
	} else {
		cli.Log.Debugf("Got unknown encryption notification from server: %s", node.XMLString())
	}
}

func (cli *Client) handleAppStateNotification(ctx context.Context, node *waBinary.Node) {
	for _, collection := range node.GetChildrenByTag("collection") {
		ag := collection.AttrGetter()
		name := appstate.WAPatchName(ag.String("name"))
		version := ag.Uint64("version")
		cli.Log.Debugf("Got server sync notification that app state %s has updated to version %d", name, version)
		err := cli.FetchAppState(ctx, name, false, false)
		if errors.Is(err, ErrIQDisconnected) || errors.Is(err, ErrNotConnected) {
			// There are some app state changes right before a remote logout, so stop syncing if we're disconnected.
			cli.Log.Debugf("Failed to sync app state after notification: %v, not trying to sync other states", err)
			return
		} else if err != nil {
			cli.Log.Errorf("Failed to sync app state after notification: %v", err)
		}
	}
}

// handlePictureNotification e' fachada: a logica vive em
// internal/wa-noise/notification (Fase F/G, lote 5).
func (cli *Client) handlePictureNotification(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	notification.HandlePicture(cli.notifT(), node)
}

func (cli *Client) handleAccountSyncNotification(ctx context.Context, node *waBinary.Node) {
	for _, child := range node.GetChildren() {
		switch child.Tag {
		case "privacy":
			cli.handlePrivacySettingsNotification(ctx, &child)
		case "devices":
			cli.handleOwnDevicesNotification(ctx, &child, node.AttrGetter().JID("from"))
		case "picture":
			cli.dispatchEvent(&events.Picture{
				Timestamp: node.AttrGetter().UnixTime("t"),
				JID:       cli.getOwnID().ToNonAD(),
			})
		case "blocklist":
			cli.handleBlocklist(ctx, &child)
		default:
			cli.Log.Debugf("Unhandled account sync item %s", child.Tag)
		}
	}
}

// handleStatusNotification e' fachada: a logica vive em
// internal/wa-noise/notification (Fase F/G, lote 5).
func (cli *Client) handleStatusNotification(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	notification.HandleStatus(cli.notifT(), node)
}

func (cli *Client) handleNotification(ctx context.Context, node *waBinary.Node) {
	ag := node.AttrGetter()
	notifType := ag.String("type")
	if !ag.OK() {
		return
	}
	var cancelled bool
	defer cli.maybeDeferredAck(ctx, node)(&cancelled)
	switch notifType {
	case notification.TypeEncrypt:
		go cli.handleEncryptNotification(ctx, node)
	case notification.TypeServerSync:
		go cli.handleAppStateNotification(ctx, node)
	case notification.TypeAccountSync:
		go cli.handleAccountSyncNotification(ctx, node)
	case notification.TypeDevices:
		cli.handleDeviceNotification(ctx, node)
	case notification.TypeFBIDDevices:
		cli.handleFBDeviceNotification(ctx, node)
	case notification.TypeGroup:
		evt, lidPairs, redactedPhones, err := cli.parseGroupNotification(node)
		if err != nil {
			cli.Log.Errorf("Failed to parse group notification: %v", err)
		} else {
			err = cli.Store.LIDs.PutManyLIDMappings(ctx, lidPairs)
			if err != nil {
				cli.Log.Errorf("Failed to store LID mappings from group notification: %v", err)
			}
			err = cli.Store.Contacts.PutManyRedactedPhones(ctx, redactedPhones)
			if err != nil {
				cli.Log.Warnf("Failed to store redacted phones from group notification: %v", err)
			}
			cancelled = cli.dispatchEvent(evt)
		}
	case notification.TypePicture:
		cli.handlePictureNotification(ctx, node)
	case notification.TypeMediaRetry:
		cli.handleMediaRetryNotification(ctx, node)
	case notification.TypePrivacyToken:
		cli.handlePrivacyTokenNotification(ctx, node)
	case notification.TypeLinkCodeCompanionReg:
		go cli.tryHandleCodePairNotification(ctx, node)
	case notification.TypeNewsletter:
		cli.handleNewsletterNotification(ctx, node)
	case notification.TypeMex:
		cli.handleMexNotification(ctx, node)
	case notification.TypeStatus:
		cli.handleStatusNotification(ctx, node)
	// Other types: business, disappearing_mode, server, status, pay, psa
	default:
		cli.Log.Debugf("Unhandled notification with type %s", notifType)
	}
}
