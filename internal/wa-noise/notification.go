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

func (cli *Client) handlePictureNotification(ctx context.Context, node *waBinary.Node) {
	ts := node.AttrGetter().UnixTime("t")
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		var evt events.Picture
		evt.Timestamp = ts
		evt.JID = ag.JID("jid")
		evt.Author = ag.OptionalJIDOrEmpty("author")
		if child.Tag == "delete" {
			evt.Remove = true
		} else if child.Tag == "add" {
			evt.PictureID = ag.String("id")
		} else if child.Tag == "set" {
			// TODO sometimes there's a hash and no ID?
			evt.PictureID = ag.String("id")
		} else {
			continue
		}
		if !ag.OK() {
			cli.Log.Debugf("Ignoring picture change notification with unexpected attributes: %v", ag.Error())
			continue
		}
		cli.dispatchEvent(&evt)
	}
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

func (cli *Client) handleStatusNotification(ctx context.Context, node *waBinary.Node) {
	ag := node.AttrGetter()
	child, found := node.GetOptionalChildByTag("set")
	if !found {
		cli.Log.Debugf("Status notifcation did not contain child with tag 'set'")
		return
	}
	status, ok := child.Content.([]byte)
	if !ok {
		cli.Log.Warnf("Set status notification has unexpected content (%T)", child.Content)
		return
	}
	cli.dispatchEvent(&events.UserAbout{
		JID:       ag.JID("from"),
		Timestamp: ag.UnixTime("t"),
		Status:    string(status),
	})
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
	case "encrypt":
		go cli.handleEncryptNotification(ctx, node)
	case "server_sync":
		go cli.handleAppStateNotification(ctx, node)
	case "account_sync":
		go cli.handleAccountSyncNotification(ctx, node)
	case "devices":
		cli.handleDeviceNotification(ctx, node)
	case "fbid:devices":
		cli.handleFBDeviceNotification(ctx, node)
	case "w:gp2":
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
	case "picture":
		cli.handlePictureNotification(ctx, node)
	case "mediaretry":
		cli.handleMediaRetryNotification(ctx, node)
	case "privacy_token":
		cli.handlePrivacyTokenNotification(ctx, node)
	case "link_code_companion_reg":
		go cli.tryHandleCodePairNotification(ctx, node)
	case "newsletter":
		cli.handleNewsletterNotification(ctx, node)
	case "mex":
		cli.handleMexNotification(ctx, node)
	case "status":
		cli.handleStatusNotification(ctx, node)
	// Other types: business, disappearing_mode, server, status, pay, psa
	default:
		cli.Log.Debugf("Unhandled notification with type %s", notifType)
	}
}
